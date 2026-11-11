package camara

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
//   - GET /eventos?idOrgao=180&dataInicio=&dataFim=   sessões do Plenário
//   - GET /eventos/{id}/pauta                         pauta de uma sessão
//   - GET /proposicoes?siglaTipo=&dataApresentacaoInicio=&dataApresentacaoFim=

// plenaryID é o id do órgão Plenário (PLEN) na API.
const plenaryID = "180"

// maxAgendaEvents limita quantas sessões têm a pauta lida numa rodada.
const maxAgendaEvents = 200

var _ app.RadarSource = (*Source)(nil)

type organ struct {
	Sigla string `json:"sigla"`
}

type event struct {
	ID             httpsrc.FlexInt `json:"id"`
	DataHoraInicio string          `json:"dataHoraInicio"`
	Situacao       string          `json:"situacao"`
	Orgaos         []organ         `json:"orgaos"`
}

type agendaProp struct {
	ID        httpsrc.FlexInt `json:"id"`
	SiglaTipo string          `json:"siglaTipo"`
	Numero    httpsrc.FlexInt `json:"numero"`
	Ano       httpsrc.FlexInt `json:"ano"`
	Ementa    string          `json:"ementa"`
}

type agendaItem struct {
	Proposicao  *agendaProp `json:"proposicao_"`
	Relacionada *agendaProp `json:"proposicaoRelacionada_"`
}

type listedProp struct {
	agendaProp
	DataApresentacao string `json:"dataApresentacao"`
}

func day(t time.Time) string { return t.In(httpsrc.Brasilia).Format(time.DateOnly) }

// Agenda implementa app.RadarSource: PL, PEC e PLP na pauta das sessões do
// Plenário, inclusive a Proposição a que um item (parecer, emenda) se
// refere. Sessão cancelada fica de fora.
func (s *Source) Agenda(ctx context.Context, from, to time.Time) ([]app.RadarHit, error) {
	u := s.c.URL("/eventos", url.Values{
		"idOrgao": {plenaryID}, "dataInicio": {day(from)}, "dataFim": {day(to)}, "itens": {"100"},
	})
	events, err := getPaged[event](ctx, s.c, u)
	if err != nil {
		return nil, fmt.Errorf("câmara: sessões do plenário: %w", err)
	}
	var out []app.RadarHit
	read := 0
	for _, e := range events {
		if e.ID <= 0 || strings.Contains(strings.ToLower(e.Situacao), "cancelad") ||
			!slices.Contains(e.Orgaos, organ{Sigla: "PLEN"}) {
			continue
		}
		at, err := httpsrc.ScheduleDate("dataHoraInicio", e.DataHoraInicio)
		if err != nil {
			return nil, fmt.Errorf("câmara: sessão %d: %w", e.ID, err)
		}
		if read++; read > maxAgendaEvents {
			return nil, fmt.Errorf("câmara: %w", httpsrc.Invalid("mais de %d sessões", maxAgendaEvents))
		}
		id := strconv.FormatInt(int64(e.ID), 10)
		var env struct {
			Dados []agendaItem `json:"dados"`
		}
		if err := s.c.GetJSON(ctx, s.c.URL("/eventos/"+id+"/pauta", nil), &env); err != nil {
			if httpsrc.IsNotFound(err) {
				continue // sessão sem pauta publicada
			}
			return nil, fmt.Errorf("câmara: pauta da sessão %s: %w", id, err)
		}
		for _, it := range env.Dados {
			for _, p := range []*agendaProp{it.Proposicao, it.Relacionada} {
				if h, ok := radarHit(p); ok {
					t := at
					h.AgendaAt = &t
					out = append(out, h)
				}
			}
		}
	}
	return out, nil
}

// Presented implementa app.RadarSource.
func (s *Source) Presented(ctx context.Context, from, to time.Time) ([]app.RadarHit, error) {
	u := s.c.URL("/proposicoes", url.Values{
		"siglaTipo": {strings.Join(app.RadarKinds, ",")}, "itens": {"100"},
		"dataApresentacaoInicio": {day(from)}, "dataApresentacaoFim": {day(to)},
	})
	list, err := getPaged[listedProp](ctx, s.c, u)
	if err != nil {
		return nil, fmt.Errorf("câmara: apresentadas: %w", err)
	}
	var out []app.RadarHit
	for _, p := range list {
		h, ok := radarHit(&p.agendaProp)
		if !ok {
			continue
		}
		at, err := httpsrc.Date("dataApresentacao", p.DataApresentacao)
		if err != nil {
			return nil, fmt.Errorf("câmara: proposição %d: %w", p.ID, err)
		}
		h.PresentedAt = &at
		out = append(out, h)
	}
	return out, nil
}

// radarHit valida uma Proposição de lista; false se não é de RadarKinds ou
// não tem nome válido.
func radarHit(p *agendaProp) (app.RadarHit, bool) {
	if p == nil || p.ID <= 0 {
		return app.RadarHit{}, false
	}
	kind, err := httpsrc.Kind(p.SiglaTipo)
	if err != nil || !slices.Contains(app.RadarKinds, kind) || !httpsrc.ValidNumber(int(p.Numero)) || !httpsrc.ValidYear(int(p.Ano)) {
		return app.RadarHit{}, false
	}
	summary, err := httpsrc.Text("ementa", p.Ementa)
	if err != nil {
		summary = ""
	}
	id := strconv.FormatInt(int64(p.ID), 10)
	return app.RadarHit{
		Chamber: app.ChamberCamara, ExternalID: id, Kind: kind, Number: int(p.Numero), Year: int(p.Ano),
		Summary: summary, SourceURL: publicPage + id,
	}, true
}
