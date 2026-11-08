package senado

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		p := strings.TrimPrefix(r.URL.Path, "/dadosabertos")
		switch {
		case p == "/processo":
			q := r.URL.Query()
			file = "busca_" + strings.ToLower(q.Get("sigla")) + "_" + q.Get("numero") + "_" + q.Get("ano") + ".json"
			if _, err := os.Stat(filepath.Join("testdata", file)); err != nil {
				file = "busca_vazia.json"
			}
		case strings.HasPrefix(p, "/processo/"):
			file = "processo_" + strings.TrimPrefix(p, "/processo/") + ".json"
		}
		b, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":404,"title":"Not Found"}`))
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
	s, err := New(srv.Client(), srv.URL+"/dadosabertos", cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func brt(layout, s string) time.Time {
	t, err := time.ParseInLocation(layout, s, httpsrc.Brasilia)
	if err != nil {
		panic(err)
	}
	return t
}

func TestSearchPEC221Revisora(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	recs, err := s.Search(context.Background(), app.SearchQuery{Kind: "PEC", Number: 221, Year: 2019})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("%d registros", len(recs))
	}
	want := app.SourceRecord{
		Chamber:         app.ChamberSenado,
		ExternalID:      "9056435",
		Kind:            "PEC",
		Number:          221,
		Year:            2019,
		OfficialSummary: "Altera o art. 7º da Constituição Federal para reduzir a duração máxima semanal do trabalho e prever 2 (dois) dias de repouso semanal remunerado, sem qualquer redução salarial; e dá outras providências.",
		PresentedAt:     brt("2006-01-02", "2026-05-28"),
		OfficialTextURL: "https://legis.senado.gov.br/sdleg-getter/dadosabertos/documento/d11bdbc4-b414-4842-bf97-882601e75cbe",
		Authors:         []string{"Câmara dos Deputados"},
		Role:            "review",
		OriginChamber:   app.ChamberCamara,
		Status:          "TRANSFORMADA EM NORMA JURÍDICA",
		SourceURL:       "https://www25.senado.leg.br/web/atividade/materias/-/materia/174386",
	}
	if !reflect.DeepEqual(recs[0], want) {
		t.Fatalf("registro\n got %+v\nwant %+v", recs[0], want)
	}
}

func TestFetchIniciadora(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	rec, err := s.Fetch(context.Background(), "8441243")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != "PL" || rec.Number != 2338 || rec.Year != 2023 {
		t.Fatalf("nome %s %d/%d", rec.Kind, rec.Number, rec.Year)
	}
	if rec.Role != "origin" || rec.OriginChamber != app.ChamberSenado {
		t.Fatalf("Role %q Origin %q", rec.Role, rec.OriginChamber)
	}
	if !reflect.DeepEqual(rec.Authors, []string{"Rodrigo Pacheco"}) {
		t.Fatalf("Authors = %q", rec.Authors)
	}
	if rec.AttachedTo != "" {
		t.Fatalf("AttachedTo = %q", rec.AttachedTo)
	}
}

func TestProgressPEC221(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	got, err := s.Progress(context.Background(), "9056435")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 {
		t.Fatalf("%d eventos", len(got))
	}
	want0 := app.SourceProgress{
		ExternalKey: "2288582",
		At:          brt("2006-01-02 15:04:05", "2026-05-28 18:42:02"),
		Body:        "PLEN",
		Description: "Autuada a Proposta de Emenda à Constituição nº 221/2019 proveniente da Câmara dos Deputados. A proposta vai à publicação.",
		Situation:   "AGUARDANDO DESPACHO",
	}
	if !reflect.DeepEqual(got[0], want0) {
		t.Fatalf("primeiro\n got %+v\nwant %+v", got[0], want0)
	}
	// Informe sem colegiado: órgão vem do ente administrativo.
	prom := got[4]
	if prom.ExternalKey != "2319976" || prom.Body != "SEXPE" || prom.Situation != "TRANSFORMADA EM NORMA JURÍDICA" ||
		!strings.Contains(prom.Description, "PROMULGADA. EMENDA CONSTITUCIONAL N° 141 DE 2026.\nDOUE") {
		t.Fatalf("promulgação = %+v", prom)
	}
	// Informe que não inicia situação.
	if last := got[5]; last.Situation != "" || last.Body != "PLEN" {
		t.Fatalf("último = %+v", last)
	}
	for i := 1; i < len(got); i++ {
		if got[i].At.Before(got[i-1].At) {
			t.Fatalf("fora de ordem em %d", i)
		}
	}
}

func TestNotFound(t *testing.T) {
	s := newSource(t, fixtures(t), Config{})
	ctx := context.Background()
	// A PEC 8/2025 nunca chegou ao Senado: busca vazia ([]).
	if _, err := s.Search(ctx, app.SearchQuery{Kind: "PEC", Number: 8, Year: 2025}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("busca vazia: %v", err)
	}
	if _, err := s.Fetch(ctx, "1"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("fetch 404: %v", err)
	}
	if _, err := s.Progress(ctx, "1"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("progress 404: %v", err)
	}
	if _, err := s.Fetch(ctx, "abc"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("ID inválido: %v", err)
	}
}

func TestSearchIgnoraResultadoAproximado(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"identificacao":"PEC 2210/2019"},{"id":2,"identificacao":"PL 221/2019"}]`))
	}
	s := newSource(t, http.HandlerFunc(h), Config{})
	if _, err := s.Search(context.Background(), app.SearchQuery{Kind: "PEC", Number: 221, Year: 2019}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("erro = %v", err)
	}
}

func TestUnavailable(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"500": func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) },
		"503 com Retry-After longo": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(503)
		},
		"lixo": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<xml/>")) },
		"grande demais": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":9056435,"x":"` + strings.Repeat("x", 4096) + `"}`))
		},
		"sem ementa": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":9056435,"sigla":"PEC","numero":"221","ano":2019,"dataInicioEfetivo":"2026-05-28"}`))
		},
		"data absurda": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"id":9056435,"sigla":"PEC","numero":"221","ano":2019,"conteudo":{"ementa":"x"},"dataInicioEfetivo":"2999-01-01"}`))
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
			if _, err := s.Fetch(context.Background(), "9056435"); !errors.Is(err, app.ErrUnavailable) {
				t.Fatalf("erro = %v", err)
			}
		})
	}
}

func TestProgressInformeRuimRejeitaTudo(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":9056435,"autuacoes":[{"situacoes":[],"informesLegislativos":[
			{"id":1,"data":"2026-05-28 18:42:02","descricao":"ok"},
			{"id":2,"data":"2026-05-28 18:42:02","descricao":"   "}]}]}`))
	}
	s := newSource(t, http.HandlerFunc(h), Config{})
	if got, err := s.Progress(context.Background(), "9056435"); !errors.Is(err, app.ErrUnavailable) || got != nil {
		t.Fatalf("got %v, erro %v", got, err)
	}
}

func TestRole(t *testing.T) {
	cases := []struct {
		objetivo, casa string
		role           string
		origin         app.ChamberCode
	}{
		{"Iniciadora", "SF", "origin", app.ChamberSenado},
		{"Revisora", "CD", "review", app.ChamberCamara},
		{"Substitutivo", "SF", "origin", app.ChamberSenado},
		{"Emenda", "CD", "review", app.ChamberCamara},
		{"", "", "", ""},
	}
	for _, c := range cases {
		r, o := role(c.objetivo, c.casa)
		if r != c.role || o != c.origin {
			t.Fatalf("role(%q,%q) = %q,%q", c.objetivo, c.casa, r, o)
		}
	}
}
