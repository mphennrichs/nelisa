package camara

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/sources/internal/httpsrc"
)

// fixtures serve as respostas reais da API gravadas em testdata/ (cortadas
// para um subconjunto representativo em 2026-11-07).
func fixtures(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != httpsrc.UserAgent {
			t.Errorf("User-Agent = %q", r.UserAgent())
		}
		var file string
		p := strings.TrimPrefix(r.URL.Path, "/api/v2")
		switch {
		case p == "/proposicoes":
			q := r.URL.Query()
			file = "busca_" + strings.ToLower(q.Get("siglaTipo")) + "_" + q.Get("numero") + "_" + q.Get("ano") + ".json"
			if _, err := os.Stat(filepath.Join("testdata", file)); err != nil {
				file = "busca_vazia.json"
			}
		case strings.HasSuffix(p, "/autores"):
			file = "autores_" + strings.Split(p, "/")[2] + ".json"
		case strings.HasSuffix(p, "/tramitacoes"):
			file = "tramitacoes_" + strings.Split(p, "/")[2] + ".json"
		case strings.HasPrefix(p, "/proposicoes/"):
			file = "proposicao_" + strings.TrimPrefix(p, "/proposicoes/") + ".json"
		}
		b, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":404,"title":"Recurso não encontrado"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}
}

func newSource(t *testing.T, h http.Handler, cfg Config) *Source {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	s, err := New(srv.Client(), srv.URL+"/api/v2", cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func brt(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02T15:04", s, httpsrc.Brasilia)
	if err != nil {
		panic(err)
	}
	return t
}

func TestSearchPEC221(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	recs, err := s.Search(context.Background(), app.SearchQuery{Kind: "pec", Number: 221, Year: 2019})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("%d registros", len(recs))
	}
	want := app.SourceRecord{
		Chamber:         app.ChamberCamara,
		ExternalID:      "2233802",
		Kind:            "PEC",
		Number:          221,
		Year:            2019,
		Summary:         "Altera o Art. 7º inciso XII da constituição Federal, reduzindo a jornada de trabalho a 36 horas semanais em 10 anos.",
		PresentedAt:     brt("2019-12-11T17:37"),
		OfficialTextURL: "https://www.camara.leg.br/proposicoesWeb/prop_mostrarintegra?codteor=1845483",
		Authors:         []string{"Reginaldo Lopes"},
		Role:            "origin",
		OriginChamber:   app.ChamberCamara,
		Status:          "Transformado em Norma Jurídica",
		SourceURL:       "https://www.camara.leg.br/proposicoesWeb/fichadetramitacao?idProposicao=2233802",
	}
	if !reflect.DeepEqual(recs[0], want) {
		t.Fatalf("registro\n got %+v\nwant %+v", recs[0], want)
	}
}

func TestFetchPEC8DesapensadaEArquivada(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	rec, err := s.Fetch(context.Background(), "2485341")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != "PEC" || rec.Number != 8 || rec.Year != 2025 {
		t.Fatalf("nome %s %d/%d", rec.Kind, rec.Number, rec.Year)
	}
	if rec.Status != "Arquivada" {
		t.Fatalf("Status = %q", rec.Status)
	}
	// Já desapensada: uriPropPrincipal é null.
	if rec.AttachedTo != "" {
		t.Fatalf("AttachedTo = %q", rec.AttachedTo)
	}
	// Só proponentes, na ordem de assinatura.
	wantAuthors := []string{"Erika Hilton", "Delegada Adriana Accorsi", "Túlio Gadêlha", "Lindbergh Farias", "Orlando Silva", "Talíria Petrone"}
	if !reflect.DeepEqual(rec.Authors, wantAuthors) {
		t.Fatalf("Authors = %q", rec.Authors)
	}
	if rec.Role != "origin" || rec.OriginChamber != app.ChamberCamara {
		t.Fatalf("Role %q Origin %q", rec.Role, rec.OriginChamber)
	}
}

func TestFetchApensada(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	rec, err := s.Fetch(context.Background(), "2623604")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != "PEC" || rec.Number != 9 || rec.Year != 2026 {
		t.Fatalf("nome %s %d/%d", rec.Kind, rec.Number, rec.Year)
	}
	if rec.AttachedTo != "PEC 32/2015" {
		t.Fatalf("AttachedTo = %q", rec.AttachedTo)
	}
}

func TestFetchVindaDoSenado(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	rec, err := s.Fetch(context.Background(), "2487262")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Role != "review" || rec.OriginChamber != app.ChamberSenado {
		t.Fatalf("Role %q Origin %q", rec.Role, rec.OriginChamber)
	}
	if !reflect.DeepEqual(rec.Authors, []string{"Senado Federal - Rodrigo Pacheco"}) {
		t.Fatalf("Authors = %q", rec.Authors)
	}
}

func TestProgressPEC221(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	got, err := s.Progress(context.Background(), "2233802")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 8 {
		t.Fatalf("%d eventos", len(got))
	}
	first := got[0]
	if first.ExternalKey != "1-100" || !first.At.Equal(brt("2019-12-11T17:37")) || first.Body != "MESA" ||
		first.Description != "Apresentação de Proposição" || first.Situation != "" ||
		!strings.HasPrefix(first.Dispatch, "Apresentação da Proposta de Emenda à Constituição n. 221/2019") {
		t.Fatalf("primeiro evento = %+v", first)
	}
	last := got[len(got)-1]
	if last.ExternalKey != "453-251" || !strings.HasPrefix(last.Dispatch, "Transformado na Emenda Constitucional 141/2026") {
		t.Fatalf("último evento = %+v", last)
	}
	keys := map[string]bool{}
	for i, p := range got {
		if keys[p.ExternalKey] {
			t.Fatalf("ExternalKey repetida %q", p.ExternalKey)
		}
		keys[p.ExternalKey] = true
		if i > 0 && p.At.Before(got[i-1].At) {
			t.Fatalf("fora de ordem em %d", i)
		}
	}
	// A sequência 407 aparece duas vezes na fonte.
	if !keys["407-195"] || !keys["407-222"] {
		t.Fatalf("chaves = %v", keys)
	}
	var toSenate bool
	for _, p := range got {
		if strings.Contains(p.Dispatch, "A matéria vai ao Senado Federal (PEC 221-C/2019)") {
			toSenate = true
		}
	}
	if !toSenate {
		t.Fatal("faltou o despacho de ida ao Senado")
	}
}

func TestProgressPEC8OrdenadoPorData(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	got, err := s.Progress(context.Background(), "2485341")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 14 {
		t.Fatalf("%d eventos", len(got))
	}
	var apensou, desapensou bool
	for i, p := range got {
		if i > 0 && p.At.Before(got[i-1].At) {
			t.Fatalf("fora de ordem em %d", i)
		}
		apensou = apensou || strings.HasPrefix(p.Dispatch, "Apense-se à(ao) PEC-221/2019")
		desapensou = desapensou || p.Description == "Desapensação"
	}
	if !apensou || !desapensou {
		t.Fatal("histórico de apensação ausente da Tramitação")
	}
}

func TestNotFound(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	ctx := context.Background()
	if _, err := s.Search(ctx, app.SearchQuery{Kind: "PEC", Number: 99999, Year: 2019}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("busca vazia: %v", err)
	}
	if _, err := s.Fetch(ctx, "1"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("fetch 404: %v", err)
	}
	if _, err := s.Progress(ctx, "1"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("progress 404: %v", err)
	}
	if _, err := s.Fetch(ctx, "../deputados"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ID inválido: %v", err)
	}
}

func TestUnavailable(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"500": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		"lixo": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("<html>manutenção</html>"))
		},
		"grande demais": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"dados":{"ementa":"` + strings.Repeat("x", 4096) + `"}}`))
		},
		"campos faltando": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"dados":{"id":2233802,"siglaTipo":"PEC","numero":221,"ano":2019}}`))
		},
		"ano absurdo": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"dados":{"id":2233802,"siglaTipo":"PEC","numero":221,"ano":3019,"ementa":"x","dataApresentacao":"2019-12-11T17:37"}}`))
		},
		"outro ID": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"dados":{"id":1,"siglaTipo":"PEC","numero":221,"ano":2019,"ementa":"x","dataApresentacao":"2019-12-11T17:37"}}`))
		},
		"lento": func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(500 * time.Millisecond):
			}
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			s := newSource(t, h, Config{MaxBody: 2048, Timeout: 50 * time.Millisecond})
			if _, err := s.Fetch(context.Background(), "2233802"); !errors.Is(err, app.ErrUnavailable) {
				t.Fatalf("erro = %v", err)
			}
		})
	}
}

func TestProgressDadoRuimRejeitaTudo(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"dados":[
			{"dataHora":"2019-12-11T17:37","sequencia":1,"siglaOrgao":"MESA","descricaoTramitacao":"ok","codTipoTramitacao":"100","despacho":"ok"},
			{"dataHora":"não é data","sequencia":2,"siglaOrgao":"MESA","descricaoTramitacao":"x","codTipoTramitacao":"100","despacho":"x"}
		],"links":[]}`))
	}
	s := newSource(t, http.HandlerFunc(h), Config{})
	if got, err := s.Progress(context.Background(), "2233802"); !errors.Is(err, app.ErrUnavailable) || got != nil {
		t.Fatalf("got %v, erro %v", got, err)
	}
}

func TestPaginacao(t *testing.T) {
	var srvURL string
	h := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pagina") == "2" {
			_, _ = w.Write([]byte(`{"dados":[{"dataHora":"2020-01-02T10:00","sequencia":2,"siglaOrgao":"CCJC","descricaoTramitacao":"b","codTipoTramitacao":"1","despacho":"b"}],"links":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"dados":[{"dataHora":"2020-01-01T10:00","sequencia":1,"siglaOrgao":"MESA","descricaoTramitacao":"a","codTipoTramitacao":"1","despacho":"a"}],
			"links":[{"rel":"next","href":"` + srvURL + `/api/v2/proposicoes/5/tramitacoes?pagina=2"},
			         {"rel":"next-externo","href":"https://evil.example/"}]}`))
	}
	srv := httptest.NewServer(http.HandlerFunc(h))
	defer srv.Close()
	srvURL = srv.URL
	s, err := New(srv.Client(), srv.URL+"/api/v2", Config{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Progress(context.Background(), "5")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Body != "CCJC" {
		t.Fatalf("got %+v", got)
	}
}

func TestPaginacaoIgnoraHostExterno(t *testing.T) {
	var calls atomic.Int32
	h := func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"dados":[{"dataHora":"2020-01-01T10:00","sequencia":1,"siglaOrgao":"MESA","descricaoTramitacao":"a","codTipoTramitacao":"1"}],
			"links":[{"rel":"next","href":"https://evil.example/api/v2/proposicoes/5/tramitacoes?pagina=2"}]}`))
	}
	s := newSource(t, http.HandlerFunc(h), Config{})
	got, err := s.Progress(context.Background(), "5")
	if err != nil || len(got) != 1 || calls.Load() != 1 {
		t.Fatalf("got %d eventos, %d chamadas, erro %v", len(got), calls.Load(), err)
	}
}
