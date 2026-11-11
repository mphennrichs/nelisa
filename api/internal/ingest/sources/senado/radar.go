package senado

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/sources/internal/httpsrc"
)

// Radar (#45). Endpoints:
//   - GET /plenario/agenda/dia/{AAAAMMDD}.json   sessões e pauta do dia
//   - GET /processo?codigoMateria=               id do processo de uma matéria
//   - GET /processo?sigla=&dataInicioApresentacao=&dataFimApresentacao=

// maxAgendaDays limita quantos dias de agenda uma rodada lê.
const maxAgendaDays = 31

var _ app.RadarSource = (*Source)(nil)

type agendaMatter struct {
	CodigoMateria httpsrc.FlexInt `json:"CodigoMateria"`
	SiglaMateria  string          `json:"SiglaMateria"`
	NumeroMateria httpsrc.FlexInt `json:"NumeroMateria"`
	AnoMateria    httpsrc.FlexInt `json:"AnoMateria"`
	Ementa        string          `json:"Ementa"`
}

type agendaSession struct {
	Data           string `json:"Data"`
	Hora           string `json:"Hora"`
	SituacaoSessao string `json:"SituacaoSessao"`
	Materias       *struct {
		Materia httpsrc.OneOrMany[agendaMatter] `json:"Materia"`
	} `json:"Materias"`
}

type listedProcess struct {
	ID               httpsrc.FlexInt `json:"id"`
	CodigoMateria    httpsrc.FlexInt `json:"codigoMateria"`
	Identificacao    string          `json:"identificacao"`
	Ementa           string          `json:"ementa"`
	DataApresentacao string          `json:"dataApresentacao"`
}

// Agenda implementa app.RadarSource: PL, PEC e PLP na pauta das sessões do
// Plenário (do Senado e do Congresso) de cada dia. A pauta traz o código
// legado da matéria; o id do processo vem de /processo?codigoMateria=.
func (s *Source) Agenda(ctx context.Context, from, to time.Time) ([]app.RadarHit, error) {
	var out []app.RadarHit
	d := from.In(httpsrc.Brasilia)
	for i := 0; !d.After(to); i, d = i+1, d.AddDate(0, 0, 1) {
		if i == maxAgendaDays {
			return nil, fmt.Errorf("senado: %w", httpsrc.Invalid("mais de %d dias de agenda", maxAgendaDays))
		}
		var env struct {
			AgendaPlenario struct {
				Sessoes *struct {
					Sessao httpsrc.OneOrMany[agendaSession] `json:"Sessao"`
				} `json:"Sessoes"`
			} `json:"AgendaPlenario"`
		}
		u := s.c.URL("/plenario/agenda/dia/"+d.Format("20060102")+".json", nil)
		if err := s.c.GetJSON(ctx, u, &env); err != nil {
			if httpsrc.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("senado: agenda de %s: %w", d.Format(time.DateOnly), err)
		}
		if env.AgendaPlenario.Sessoes == nil {
			continue
		}
		for _, sess := range env.AgendaPlenario.Sessoes.Sessao {
			if sess.Materias == nil || strings.Contains(strings.ToLower(sess.SituacaoSessao), "cancelad") {
				continue
			}
			at, err := httpsrc.ScheduleDate("data da sessão", strings.TrimSpace(sess.Data)+" "+strings.TrimSpace(sess.Hora))
			if err != nil {
				if at, err = httpsrc.ScheduleDate("data da sessão", sess.Data); err != nil {
					return nil, fmt.Errorf("senado: %w", err)
				}
			}
			for _, m := range sess.Materias.Materia {
				kind, err := httpsrc.Kind(m.SiglaMateria)
				if err != nil || !slices.Contains(app.RadarKinds, kind) || m.CodigoMateria <= 0 {
					continue
				}
				id, err := s.processID(ctx, int64(m.CodigoMateria))
				if err != nil {
					return nil, err
				}
				if id == "" {
					continue
				}
				h, ok := radarHit(id, int64(m.CodigoMateria), kind, int(m.NumeroMateria), int(m.AnoMateria), m.Ementa)
				if !ok {
					continue
				}
				t := at
				h.AgendaAt = &t
				out = append(out, h)
			}
		}
	}
	return out, nil
}

// processID devolve o id do processo de uma matéria ("" se não há).
func (s *Source) processID(ctx context.Context, codigoMateria int64) (string, error) {
	var list []listedProcess
	u := s.c.URL("/processo", url.Values{"codigoMateria": {strconv.FormatInt(codigoMateria, 10)}})
	if err := s.c.GetJSON(ctx, u, &list); err != nil {
		if httpsrc.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("senado: processo da matéria %d: %w", codigoMateria, err)
	}
	for _, p := range list {
		if int64(p.CodigoMateria) == codigoMateria && p.ID > 0 {
			return strconv.FormatInt(int64(p.ID), 10), nil
		}
	}
	return "", nil
}

// Presented implementa app.RadarSource: uma consulta por Tipo.
func (s *Source) Presented(ctx context.Context, from, to time.Time) ([]app.RadarHit, error) {
	var out []app.RadarHit
	for _, kind := range app.RadarKinds {
		u := s.c.URL("/processo", url.Values{
			"sigla":                  {kind},
			"dataInicioApresentacao": {from.In(httpsrc.Brasilia).Format(time.DateOnly)},
			"dataFimApresentacao":    {to.In(httpsrc.Brasilia).Format(time.DateOnly)},
		})
		var list []listedProcess
		if err := s.c.GetJSON(ctx, u, &list); err != nil {
			if httpsrc.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("senado: %s apresentadas: %w", kind, err)
		}
		for _, p := range list {
			m := identRe.FindStringSubmatch(strings.TrimSpace(p.Identificacao))
			if m == nil || m[1] != kind || p.ID <= 0 {
				continue
			}
			n, _ := strconv.Atoi(m[2])
			y, _ := strconv.Atoi(m[3])
			h, ok := radarHit(strconv.FormatInt(int64(p.ID), 10), int64(p.CodigoMateria), kind, n, y, p.Ementa)
			if !ok {
				continue
			}
			at, err := httpsrc.Date("dataApresentacao", p.DataApresentacao)
			if err != nil {
				return nil, fmt.Errorf("senado: processo %d: %w", p.ID, err)
			}
			h.PresentedAt = &at
			out = append(out, h)
		}
	}
	return out, nil
}

func radarHit(id string, codigoMateria int64, kind string, number, year int, ementa string) (app.RadarHit, bool) {
	if !httpsrc.ValidNumber(number) || !httpsrc.ValidYear(year) {
		return app.RadarHit{}, false
	}
	summary, err := httpsrc.Text("ementa", ementa)
	if err != nil {
		summary = ""
	}
	h := app.RadarHit{Chamber: app.ChamberSenado, ExternalID: id, Kind: kind, Number: number, Year: year, Summary: summary}
	if codigoMateria > 0 {
		h.SourceURL = publicPage + strconv.FormatInt(codigoMateria, 10)
	}
	return h, true
}
