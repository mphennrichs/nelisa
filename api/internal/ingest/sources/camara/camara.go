// Package camara implementa app.Source sobre a API de Dados Abertos da
// Câmara dos Deputados (v2, JSON).
//
// Endpoints usados:
//   - GET /proposicoes?siglaTipo=&numero=&ano=      busca pelo nome oficial
//   - GET /proposicoes/{id}                         dados da Proposição
//   - GET /proposicoes/{id}/autores                 autores
//   - GET /proposicoes/{id}/tramitacoes             Tramitação
//
// A Apensação vem de uriPropPrincipal, que só é preenchido enquanto a
// Proposição está apensada; o histórico (apensou, desapensou) aparece apenas
// como texto na Tramitação e não é interpretado aqui.
package camara

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
	// DefaultBaseURL é a raiz da API v2 da Câmara.
	DefaultBaseURL = "https://dadosabertos.camara.leg.br/api/v2"
	// publicPage é a ficha de tramitação, para o Leitor conferir.
	publicPage = "https://www.camara.leg.br/proposicoesWeb/fichadetramitacao?idProposicao="
	// maxPages limita a paginação seguida por links rel=next.
	maxPages = 50
	// maxSearchResults limita quantos resultados da busca viram Fetch.
	maxSearchResults = 5
	// maxAuthors protege contra listas absurdas (PECs têm ~200 assinaturas).
	maxAuthors = 1000
)

// Config ajusta timeout, concorrência e tamanho máximo de resposta; zerado
// usa os padrões (15 s, 2 simultâneas, 10 MiB).
type Config = httpsrc.Config

// Source é a fonte oficial da Câmara dos Deputados.
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
		return nil, fmt.Errorf("câmara: %w", err)
	}
	return &Source{c: c}, nil
}

// Chamber implementa app.Source.
func (s *Source) Chamber() app.ChamberCode { return app.ChamberCamara }

// --- formatos da API ---

type link struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

type searchItem struct {
	ID        httpsrc.FlexInt `json:"id"`
	SiglaTipo string          `json:"siglaTipo"`
	Numero    httpsrc.FlexInt `json:"numero"`
	Ano       httpsrc.FlexInt `json:"ano"`
}

type proposition struct {
	ID               httpsrc.FlexInt `json:"id"`
	SiglaTipo        string          `json:"siglaTipo"`
	Numero           httpsrc.FlexInt `json:"numero"`
	Ano              httpsrc.FlexInt `json:"ano"`
	Ementa           string          `json:"ementa"`
	DataApresentacao string          `json:"dataApresentacao"`
	URLInteiroTeor   *string         `json:"urlInteiroTeor"`
	UriPropPrincipal *string         `json:"uriPropPrincipal"`
	Status           *struct {
		DescricaoSituacao *string `json:"descricaoSituacao"`
	} `json:"statusProposicao"`
}

type author struct {
	Nome            string          `json:"nome"`
	Tipo            string          `json:"tipo"`
	OrdemAssinatura httpsrc.FlexInt `json:"ordemAssinatura"`
	Proponente      httpsrc.FlexInt `json:"proponente"`
}

type progressItem struct {
	DataHora            string          `json:"dataHora"`
	Sequencia           httpsrc.FlexInt `json:"sequencia"`
	SiglaOrgao          *string         `json:"siglaOrgao"`
	DescricaoTramitacao *string         `json:"descricaoTramitacao"`
	CodTipoTramitacao   *string         `json:"codTipoTramitacao"`
	Despacho            *string         `json:"despacho"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// --- Search ---

// Search implementa app.Source. Sem resultado devolve ErrNotFound.
func (s *Source) Search(ctx context.Context, q app.SearchQuery) ([]app.SourceRecord, error) {
	q, err := httpsrc.NormalizeQuery(q)
	if err != nil {
		return nil, err
	}
	u := s.c.URL("/proposicoes", url.Values{
		"siglaTipo": {q.Kind},
		"numero":    {strconv.Itoa(q.Number)},
		"ano":       {strconv.Itoa(q.Year)},
	})
	items, err := getPaged[searchItem](ctx, s.c, u)
	if err != nil {
		return nil, fmt.Errorf("câmara: busca %s %d/%d: %w", q.Kind, q.Number, q.Year, err)
	}
	var ids []string
	for _, it := range items {
		// A API pode trazer resultados aproximados: só aceitamos o nome exato.
		if strings.EqualFold(strings.TrimSpace(it.SiglaTipo), q.Kind) &&
			int(it.Numero) == q.Number && int(it.Ano) == q.Year && it.ID > 0 {
			ids = append(ids, strconv.FormatInt(int64(it.ID), 10))
		}
		if len(ids) == maxSearchResults {
			break
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("câmara: %w: %s %d/%d", app.ErrNotFound, q.Kind, q.Number, q.Year)
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
func (s *Source) Fetch(ctx context.Context, externalID string) (app.SourceRecord, error) {
	id, err := httpsrc.ExternalID(externalID)
	if err != nil {
		return app.SourceRecord{}, fmt.Errorf("câmara: %w", err)
	}
	p, err := s.proposition(ctx, id)
	if err != nil {
		return app.SourceRecord{}, err
	}
	rec, err := mapProposition(id, p)
	if err != nil {
		return app.SourceRecord{}, fmt.Errorf("câmara: proposição %s: %w", id, err)
	}

	authors, err := getPaged[author](ctx, s.c, s.c.URL("/proposicoes/"+id+"/autores", nil))
	if err != nil {
		return app.SourceRecord{}, fmt.Errorf("câmara: autores de %s: %w", id, subresource(err))
	}
	rec.Authors, err = mapAuthors(authors)
	if err != nil {
		return app.SourceRecord{}, fmt.Errorf("câmara: autores de %s: %w", id, err)
	}
	rec.Role, rec.OriginChamber = role(rec.Authors)

	if pid := principalID(str(p.UriPropPrincipal)); pid != "" && pid != id {
		pp, err := s.proposition(ctx, pid)
		if err != nil {
			return app.SourceRecord{}, fmt.Errorf("câmara: principal de %s: %w", id, subresource(err))
		}
		kind, err := httpsrc.Kind(pp.SiglaTipo)
		if err != nil || !httpsrc.ValidNumber(int(pp.Numero)) || !httpsrc.ValidYear(int(pp.Ano)) {
			return app.SourceRecord{}, fmt.Errorf("câmara: principal de %s: %w", id, httpsrc.Invalid("nome da principal %q %d/%d", pp.SiglaTipo, pp.Numero, pp.Ano))
		}
		rec.AttachedTo = fmt.Sprintf("%s %d/%d", kind, pp.Numero, pp.Ano)
	}
	return rec, nil
}

func (s *Source) proposition(ctx context.Context, id string) (proposition, error) {
	var env struct {
		Dados *proposition `json:"dados"`
	}
	if err := s.c.GetJSON(ctx, s.c.URL("/proposicoes/"+id, nil), &env); err != nil {
		return proposition{}, fmt.Errorf("câmara: proposição %s: %w", id, err)
	}
	if env.Dados == nil {
		return proposition{}, fmt.Errorf("câmara: proposição %s: %w", id, app.ErrNotFound)
	}
	if strconv.FormatInt(int64(env.Dados.ID), 10) != id {
		return proposition{}, fmt.Errorf("câmara: proposição %s: %w", id, httpsrc.Invalid("ID %d diferente do pedido", env.Dados.ID))
	}
	return *env.Dados, nil
}

// subresource: um 404 num sub-recurso de uma Proposição que existe é
// inconsistência da fonte, não "não encontrada".
func subresource(err error) error {
	if httpsrc.IsNotFound(err) {
		return httpsrc.Invalid("sub-recurso sumiu: %v", err)
	}
	return err
}

func mapProposition(id string, p proposition) (app.SourceRecord, error) {
	kind, err := httpsrc.Kind(p.SiglaTipo)
	if err != nil {
		return app.SourceRecord{}, err
	}
	if !httpsrc.ValidNumber(int(p.Numero)) || !httpsrc.ValidYear(int(p.Ano)) {
		return app.SourceRecord{}, httpsrc.Invalid("número/ano %d/%d", p.Numero, p.Ano)
	}
	summary, err := httpsrc.Text("ementa", p.Ementa)
	if err != nil {
		return app.SourceRecord{}, err
	}
	if summary == "" {
		return app.SourceRecord{}, httpsrc.Invalid("ementa vazia")
	}
	presented, err := httpsrc.Date("dataApresentacao", p.DataApresentacao)
	if err != nil {
		return app.SourceRecord{}, err
	}
	var status string
	if p.Status != nil {
		if status, err = httpsrc.Line("situação", str(p.Status.DescricaoSituacao)); err != nil {
			return app.SourceRecord{}, err
		}
	}
	return app.SourceRecord{
		Chamber:         app.ChamberCamara,
		ExternalID:      id,
		Kind:            kind,
		Number:          int(p.Numero),
		Year:            int(p.Ano),
		Summary:         summary,
		PresentedAt:     presented,
		OfficialTextURL: httpsrc.OfficialURL(str(p.URLInteiroTeor)),
		Status:          status,
		SourceURL:       publicPage + id,
	}, nil
}

// mapAuthors devolve os proponentes na ordem de assinatura. Em PECs a Câmara
// lista também assinaturas que não contam como proponente (proponente=0);
// essas ficam de fora, a menos que não haja nenhum proponente marcado.
func mapAuthors(in []author) ([]string, error) {
	if len(in) > maxAuthors {
		return nil, httpsrc.Invalid("%d autores", len(in))
	}
	sort.SliceStable(in, func(i, j int) bool { return in[i].OrdemAssinatura < in[j].OrdemAssinatura })
	hasProponent := false
	for _, a := range in {
		if a.Proponente == 1 {
			hasProponent = true
			break
		}
	}
	var out []string
	for _, a := range in {
		if hasProponent && a.Proponente != 1 {
			continue
		}
		name, err := httpsrc.Line("autor", a.Nome)
		if err != nil {
			return nil, err
		}
		if name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}

// role deduz o papel da Câmara. Proposição vinda do Senado tem como autor o
// órgão "Senado Federal" (a Câmara escreve "Senado Federal - <senador>");
// qualquer outra autoria (deputado, comissão, Executivo, STF, iniciativa
// popular) começa na Câmara (CF, art. 64).
func role(authors []string) (string, app.ChamberCode) {
	for _, a := range authors {
		if strings.HasPrefix(strings.ToLower(a), "senado federal") {
			return "review", app.ChamberSenado
		}
	}
	return "origin", app.ChamberCamara
}

var principalRe = regexp.MustCompile(`/proposicoes/([0-9]{1,15})/?$`)

func principalID(uri string) string {
	m := principalRe.FindStringSubmatch(strings.TrimSpace(uri))
	if m == nil {
		return ""
	}
	return m[1]
}

// --- Progress ---

// Progress implementa app.Source.
//
// ExternalKey é "<sequencia>-<codTipoTramitacao>": a Câmara repete a
// sequência em eventos diferentes (a PEC 221/2019 tem dois eventos 407 no
// mesmo minuto). Se ainda assim houver repetição, os seguintes ganham "#2",
// "#3"... na ordem da resposta.
//
// Situation fica vazio: o descricaoSituacao de cada tramitação não é a
// situação depois do evento (o evento 1 da PEC 221/2019, a apresentação,
// vem com "Transformado em Norma Jurídica"). A situação atual está em
// SourceRecord.Status.
func (s *Source) Progress(ctx context.Context, externalID string) ([]app.SourceProgress, error) {
	id, err := httpsrc.ExternalID(externalID)
	if err != nil {
		return nil, fmt.Errorf("câmara: %w", err)
	}
	items, err := getPaged[progressItem](ctx, s.c, s.c.URL("/proposicoes/"+id+"/tramitacoes", nil))
	if err != nil {
		return nil, fmt.Errorf("câmara: tramitação de %s: %w", id, err)
	}
	out, err := mapProgress(items)
	if err != nil {
		return nil, fmt.Errorf("câmara: tramitação de %s: %w", id, err)
	}
	return out, nil
}

func mapProgress(items []progressItem) ([]app.SourceProgress, error) {
	type row struct {
		p   app.SourceProgress
		seq int64
	}
	rows := make([]row, 0, len(items))
	seen := map[string]int{}
	for i, it := range items {
		if it.Sequencia <= 0 {
			return nil, httpsrc.Invalid("tramitação %d sem sequência", i)
		}
		at, err := httpsrc.Date("dataHora", it.DataHora)
		if err != nil {
			return nil, err
		}
		body, err := httpsrc.Line("siglaOrgao", str(it.SiglaOrgao))
		if err != nil {
			return nil, err
		}
		desc, err := httpsrc.Line("descricaoTramitacao", str(it.DescricaoTramitacao))
		if err != nil {
			return nil, err
		}
		dispatch, err := httpsrc.Text("despacho", str(it.Despacho))
		if err != nil {
			return nil, err
		}
		if desc == "" && dispatch == "" {
			return nil, httpsrc.Invalid("tramitação %d sem descrição", it.Sequencia)
		}
		code, err := httpsrc.Line("codTipoTramitacao", str(it.CodTipoTramitacao))
		if err != nil {
			return nil, err
		}
		key := strconv.FormatInt(int64(it.Sequencia), 10) + "-" + code
		seen[key]++
		if n := seen[key]; n > 1 {
			key += "#" + strconv.Itoa(n)
		}
		rows = append(rows, row{
			p: app.SourceProgress{
				ExternalKey: key,
				At:          at,
				Body:        body,
				Description: desc,
				Dispatch:    dispatch,
			},
			seq: int64(it.Sequencia),
		})
	}
	// A API ordena por data, mas não promete: do mais antigo ao mais novo,
	// desempatando pela sequência.
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].p.At.Equal(rows[j].p.At) {
			return rows[i].p.At.Before(rows[j].p.At)
		}
		return rows[i].seq < rows[j].seq
	})
	out := make([]app.SourceProgress, len(rows))
	for i, r := range rows {
		out[i] = r.p
	}
	return out, nil
}

// getPaged lê "dados" de u e das páginas seguintes (links rel=next, só no
// mesmo host). Hoje busca, autores e tramitações vêm numa página só, mas a
// API documenta paginação por pagina/itens.
func getPaged[T any](ctx context.Context, c *httpsrc.Client, u string) ([]T, error) {
	var all []T
	visited := map[string]bool{}
	for page := 0; u != ""; page++ {
		if page == maxPages {
			return nil, httpsrc.Invalid("mais de %d páginas", maxPages)
		}
		if visited[u] {
			return nil, httpsrc.Invalid("paginação em ciclo em %s", u)
		}
		visited[u] = true
		var env struct {
			Dados []T    `json:"dados"`
			Links []link `json:"links"`
		}
		if err := c.GetJSON(ctx, u, &env); err != nil {
			return nil, err
		}
		all = append(all, env.Dados...)
		u = ""
		for _, l := range env.Links {
			if l.Rel == "next" && c.SameHost(l.Href) {
				u = l.Href
			}
		}
	}
	return all, nil
}
