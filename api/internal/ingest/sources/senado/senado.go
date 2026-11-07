// Package senado implementa app.Source sobre a API de Dados Abertos
// Legislativos do Senado Federal (modelo "processo", JSON).
//
// Endpoints usados (documentados e não depreciados; os /materia/* estão em
// descontinuação):
//   - GET /processo?sigla=&numero=&ano=   busca pelo nome oficial
//   - GET /processo/{id}                  dados do processo e Tramitação
//     (autuacoes[].informesLegislativos), numa chamada só
//
// ExternalID é o "id" do processo, e não o "codigoMateria": é a chave do
// modelo novo e a única aceita por /processo/{id}; o codigoMateria é o
// código legado do sistema antigo (MATE), usado só para montar o link da
// página pública.
package senado

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/sources/internal/httpsrc"
)

const (
	// DefaultBaseURL é a raiz da API de Dados Abertos do Senado.
	DefaultBaseURL = "https://legis.senado.leg.br/dadosabertos"
	// publicPage é a página da matéria, para o Leitor conferir.
	publicPage = "https://www25.senado.leg.br/web/atividade/materias/-/materia/"
	// maxSearchResults limita quantos resultados da busca viram Fetch.
	maxSearchResults = 5
	// maxItems protege contra listas absurdas (autores, informes).
	maxItems = 20_000
)

// Config ajusta timeout, concorrência e tamanho máximo de resposta; zerado
// usa os padrões (15 s, 2 simultâneas, 10 MiB). A API do Senado devolve 429
// acima de 10 requisições por segundo.
type Config = httpsrc.Config

// Source é a fonte oficial do Senado Federal.
type Source struct {
	c *httpsrc.Client
}

var _ app.Source = (*Source)(nil)

// New cria a fonte. hc nil usa um http.Client próprio; baseURL "" usa
// DefaultBaseURL.
func New(hc *http.Client, baseURL string, cfg Config) (*Source, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	c, err := httpsrc.New(hc, strings.TrimRight(baseURL, "/"), cfg)
	if err != nil {
		return nil, fmt.Errorf("senado: %w", err)
	}
	return &Source{c: c}, nil
}

// Chamber implementa app.Source.
func (s *Source) Chamber() app.ChamberCode { return app.ChamberSenado }

// --- formatos da API ---

type searchItem struct {
	ID            httpsrc.FlexInt `json:"id"`
	Identificacao string          `json:"identificacao"`
}

type ref struct {
	Sigla string `json:"sigla"`
}

type process struct {
	ID                  httpsrc.FlexInt `json:"id"`
	CodigoMateria       httpsrc.FlexInt `json:"codigoMateria"`
	Sigla               string          `json:"sigla"`
	Numero              httpsrc.FlexInt `json:"numero"`
	Ano                 httpsrc.FlexInt `json:"ano"`
	Objetivo            string          `json:"objetivo"`
	SiglaCasaIniciadora string          `json:"siglaCasaIniciadora"`
	SituacaoAtual       string          `json:"situacaoAtual"`
	DataInicioEfetivo   string          `json:"dataInicioEfetivo"`
	Conteudo            *struct {
		Ementa string `json:"ementa"`
	} `json:"conteudo"`
	Documento *struct {
		DataApresentacao string `json:"dataApresentacao"`
		URL              string `json:"url"`
		ResumoAutoria    string `json:"resumoAutoria"`
		Autoria          []struct {
			Autor string          `json:"autor"`
			Ordem httpsrc.FlexInt `json:"ordem"`
		} `json:"autoria"`
	} `json:"documento"`
	Autuacoes []struct {
		Situacoes []struct {
			IDTipo    httpsrc.FlexInt `json:"idTipo"`
			Descricao string          `json:"descricao"`
		} `json:"situacoes"`
		Informes []struct {
			ID                 httpsrc.FlexInt `json:"id"`
			Data               string          `json:"data"`
			Colegiado          *ref            `json:"colegiado"`
			EnteAdministrativo *ref            `json:"enteAdministrativo"`
			Descricao          string          `json:"descricao"`
			IDSituacaoIniciada httpsrc.FlexInt `json:"idSituacaoIniciada"`
		} `json:"informesLegislativos"`
	} `json:"autuacoes"`
}

// --- Search ---

// identRe lê "PEC 221/2019" e também "PL 282/2024 (Emenda-CD)".
var identRe = regexp.MustCompile(`^([A-Z]{1,12})\s+0*([0-9]{1,7})/([0-9]{4})\b`)

// Search implementa app.Source. Sem resultado devolve ErrNotFound.
//
// Desde 2019 o Senado mantém a identificação da Casa iniciadora (a PEC
// 221/2019 da Câmara é "PEC 221/2019" aqui também). Proposições mais antigas
// podem ter outro número em cada Casa; por isso o casamento entre Casas é
// sugerido e confirmado pelo Editor (ADR 0006), nunca feito aqui.
func (s *Source) Search(ctx context.Context, q app.SearchQuery) ([]app.SourceRecord, error) {
	q, err := httpsrc.NormalizeQuery(q)
	if err != nil {
		return nil, err
	}
	u := s.c.URL("/processo", url.Values{
		"sigla":  {q.Kind},
		"numero": {strconv.Itoa(q.Number)},
		"ano":    {strconv.Itoa(q.Year)},
	})
	var items []searchItem
	if err := s.c.GetJSON(ctx, u, &items); err != nil {
		return nil, fmt.Errorf("senado: busca %s %d/%d: %w", q.Kind, q.Number, q.Year, err)
	}
	var ids []string
	for _, it := range items {
		m := identRe.FindStringSubmatch(strings.TrimSpace(it.Identificacao))
		if m == nil || it.ID <= 0 {
			continue
		}
		n, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		if m[1] == q.Kind && n == q.Number && y == q.Year {
			ids = append(ids, strconv.FormatInt(int64(it.ID), 10))
		}
		if len(ids) == maxSearchResults {
			break
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("senado: %w: %s %d/%d", app.ErrNotFound, q.Kind, q.Number, q.Year)
	}
	out := make([]app.SourceRecord, 0, len(ids))
	for _, id := range ids {
		rec, err := s.Fetch(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// --- Fetch ---

// Fetch implementa app.Source.
//
// AttachedTo fica sempre vazio: a tramitação conjunta do Senado não tem
// principal e não aparece de forma estruturada em /processo
// (processosRelacionados só traz REFERENCIA e afins); está apenas no texto
// dos informes.
func (s *Source) Fetch(ctx context.Context, externalID string) (app.SourceRecord, error) {
	id, p, err := s.process(ctx, externalID)
	if err != nil {
		return app.SourceRecord{}, err
	}
	rec, err := mapProcess(id, p)
	if err != nil {
		return app.SourceRecord{}, fmt.Errorf("senado: processo %s: %w", id, err)
	}
	return rec, nil
}

func (s *Source) process(ctx context.Context, externalID string) (string, process, error) {
	id, err := httpsrc.ExternalID(externalID)
	if err != nil {
		return "", process{}, fmt.Errorf("senado: %w", err)
	}
	var p process
	if err := s.c.GetJSON(ctx, s.c.URL("/processo/"+id, nil), &p); err != nil {
		return "", process{}, fmt.Errorf("senado: processo %s: %w", id, err)
	}
	if strconv.FormatInt(int64(p.ID), 10) != id {
		return "", process{}, fmt.Errorf("senado: processo %s: %w", id, httpsrc.Invalid("ID %d diferente do pedido", p.ID))
	}
	return id, p, nil
}

func mapProcess(id string, p process) (app.SourceRecord, error) {
	kind, err := httpsrc.Kind(p.Sigla)
	if err != nil {
		return app.SourceRecord{}, err
	}
	if !httpsrc.ValidNumber(int(p.Numero)) || !httpsrc.ValidYear(int(p.Ano)) {
		return app.SourceRecord{}, httpsrc.Invalid("número/ano %d/%d", p.Numero, p.Ano)
	}
	if p.Conteudo == nil {
		return app.SourceRecord{}, httpsrc.Invalid("sem conteúdo (ementa)")
	}
	summary, err := httpsrc.Text("ementa", p.Conteudo.Ementa)
	if err != nil {
		return app.SourceRecord{}, err
	}
	if summary == "" {
		return app.SourceRecord{}, httpsrc.Invalid("ementa vazia")
	}
	presentedRaw := p.DataInicioEfetivo
	var textURL string
	var authors []string
	if d := p.Documento; d != nil {
		if strings.TrimSpace(d.DataApresentacao) != "" {
			presentedRaw = d.DataApresentacao
		}
		textURL = httpsrc.OfficialURL(d.URL)
		if len(d.Autoria) > maxItems {
			return app.SourceRecord{}, httpsrc.Invalid("%d autores", len(d.Autoria))
		}
		sort.SliceStable(d.Autoria, func(i, j int) bool { return d.Autoria[i].Ordem < d.Autoria[j].Ordem })
		for _, a := range d.Autoria {
			name, err := httpsrc.Line("autor", a.Autor)
			if err != nil {
				return app.SourceRecord{}, err
			}
			if name != "" {
				authors = append(authors, name)
			}
		}
		if len(authors) == 0 {
			name, err := httpsrc.Line("resumoAutoria", d.ResumoAutoria)
			if err != nil {
				return app.SourceRecord{}, err
			}
			if name != "" {
				authors = []string{name}
			}
		}
	}
	presented, err := httpsrc.Date("dataApresentacao", presentedRaw)
	if err != nil {
		return app.SourceRecord{}, err
	}
	status, err := httpsrc.Line("situacaoAtual", p.SituacaoAtual)
	if err != nil {
		return app.SourceRecord{}, err
	}
	role, origin := role(p.Objetivo, p.SiglaCasaIniciadora)
	var sourceURL string
	if p.CodigoMateria > 0 {
		sourceURL = publicPage + strconv.FormatInt(int64(p.CodigoMateria), 10)
	}
	return app.SourceRecord{
		Chamber:         app.ChamberSenado,
		ExternalID:      id,
		Kind:            kind,
		Number:          int(p.Numero),
		Year:            int(p.Ano),
		Summary:         summary,
		PresentedAt:     presented,
		OfficialTextURL: textURL,
		Authors:         authors,
		Role:            role,
		OriginChamber:   origin,
		Status:          status,
		SourceURL:       sourceURL,
	}, nil
}

// role usa siglaCasaIniciadora ("CD"/"SF") para a Casa iniciadora e
// "objetivo" ("Iniciadora"/"Revisora") para o papel. O objetivo também
// pode ser "Substitutivo" ou "Emenda" (texto da Câmara voltando ao Senado
// iniciador); aí o papel sai da Casa iniciadora.
func role(objetivo, casaIniciadora string) (string, app.ChamberCode) {
	var origin app.ChamberCode
	switch strings.ToUpper(strings.TrimSpace(casaIniciadora)) {
	case "SF":
		origin = app.ChamberSenado
	case "CD":
		origin = app.ChamberCamara
	}
	switch strings.ToLower(strings.TrimSpace(objetivo)) {
	case "iniciadora":
		return "origin", origin
	case "revisora":
		return "review", origin
	}
	switch origin {
	case app.ChamberSenado:
		return "origin", origin
	case app.ChamberCamara:
		return "review", origin
	}
	return "", origin
}

// --- Progress ---

// Progress implementa app.Source com os informes legislativos de todas as
// autuações do processo. ExternalKey é o id do informe (único e estável).
// Situation é a descrição da situação que o informe iniciou, quando há.
// O texto do informe vai em Description; o Senado não separa despacho.
func (s *Source) Progress(ctx context.Context, externalID string) ([]app.SourceProgress, error) {
	id, p, err := s.process(ctx, externalID)
	if err != nil {
		return nil, err
	}
	out, err := mapProgress(p)
	if err != nil {
		return nil, fmt.Errorf("senado: tramitação de %s: %w", id, err)
	}
	return out, nil
}

func mapProgress(p process) ([]app.SourceProgress, error) {
	type row struct {
		p  app.SourceProgress
		id int64
	}
	var rows []row
	seen := map[int64]bool{}
	for _, a := range p.Autuacoes {
		situations := map[int64]string{}
		for _, st := range a.Situacoes {
			d, err := httpsrc.Line("situação", st.Descricao)
			if err != nil {
				return nil, err
			}
			situations[int64(st.IDTipo)] = d
		}
		for _, inf := range a.Informes {
			if len(rows) >= maxItems {
				return nil, httpsrc.Invalid("mais de %d informes", maxItems)
			}
			if inf.ID <= 0 {
				return nil, httpsrc.Invalid("informe sem id")
			}
			if seen[int64(inf.ID)] {
				continue
			}
			seen[int64(inf.ID)] = true
			at, err := httpsrc.Date("data do informe", inf.Data)
			if err != nil {
				return nil, err
			}
			desc, err := httpsrc.Text("descrição do informe", inf.Descricao)
			if err != nil {
				return nil, err
			}
			if desc == "" {
				return nil, httpsrc.Invalid("informe %d sem descrição", inf.ID)
			}
			var body string
			if inf.Colegiado != nil {
				body = inf.Colegiado.Sigla
			}
			if strings.TrimSpace(body) == "" && inf.EnteAdministrativo != nil {
				body = inf.EnteAdministrativo.Sigla
			}
			if body, err = httpsrc.Line("órgão", body); err != nil {
				return nil, err
			}
			rows = append(rows, row{
				p: app.SourceProgress{
					ExternalKey: strconv.FormatInt(int64(inf.ID), 10),
					At:          at,
					Body:        body,
					Description: desc,
					Situation:   situations[int64(inf.IDSituacaoIniciada)],
				},
				id: int64(inf.ID),
			})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].p.At.Equal(rows[j].p.At) {
			return rows[i].p.At.Before(rows[j].p.At)
		}
		return rows[i].id < rows[j].id
	})
	out := make([]app.SourceProgress, len(rows))
	for i, r := range rows {
		out[i] = r.p
	}
	return out, nil
}
