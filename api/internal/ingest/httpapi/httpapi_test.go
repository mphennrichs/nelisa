package httpapi_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/ingest/httpapi"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
)

type server struct {
	w      *fakes.World
	camara *fakes.Source
	radar  *fakes.Radar
	sched  *fakes.RadarScheduler
	scan   *app.ScanRadar
	h      http.Handler
}

func newServer() *server {
	w := fakes.NewWorld()
	camara, senado := fakes.NewSource(app.ChamberCamara), fakes.NewSource(app.ChamberSenado)
	camara.Put(app.SourceRecord{ExternalID: "2192459", Kind: "PEC", Number: 221, Year: 2019, OfficialSummary: "Altera a jornada",
		PresentedAt: time.Date(2019, 11, 26, 0, 0, 0, 0, time.UTC)},
		fakes.Progress1("1-100", time.Date(2019, 11, 26, 0, 0, 0, 0, time.UTC), "Apresentação"))
	sources := app.NewSources(camara, senado)
	radar, sched := fakes.NewRadar(), &fakes.RadarScheduler{}
	clk := fixedClock{time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)}
	uc := httpapi.UseCases{
		SearchSources: app.NewSearchSources(sources, w.Catalog()),
		Import:        app.NewImport(sources, w.Catalog(), w.Progress(), w.Tx(), w.Audit()),
		ListRadar:     app.NewListRadar(radar, w.Catalog(), clk),
		DismissRadar:  app.NewDismissRadarItem(radar, w.Tx(), w.Audit(), clk),
		RequestRadar:  app.NewRequestRadarScan(sched),
	}
	h := httpapi.New(uc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.Route("/api/team", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				if v := req.Header.Get("X-Test-Actor"); v != "" {
					id, role, _ := strings.Cut(v, ":")
					req = req.WithContext(auth.WithActor(req.Context(), auth.Actor{UserID: id, Role: auth.Role(role)}))
				}
				next.ServeHTTP(rw, req)
			})
		})
		h.RegisterTeam(r)
	})
	return &server{w: w, camara: camara, radar: radar, sched: sched, scan: app.NewScanRadar(sources, radar, clk), h: r}
}

func (s *server) do(method, path, body, actor string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if actor != "" {
		r.Header.Set("X-Test-Actor", actor)
	}
	w := httptest.NewRecorder()
	s.h.ServeHTTP(w, r)
	return w
}

func code(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var e httpx.ErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("not the standard error body: %s", w.Body)
	}
	return e.Error.Code
}

const ed = "editor-1:editor"

func TestAuthorization(t *testing.T) {
	s := newServer()
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/team/sources/search?kind=PEC&number=221&year=2019", ""},
		{"POST", "/api/team/propositions/import", `{"chamber":"camara","external_id":"2192459"}`},
	} {
		if w := s.do(c.method, c.path, c.body, ""); w.Code != 401 {
			t.Errorf("%s anonymous: %d", c.path, w.Code)
		}
		if w := s.do(c.method, c.path, c.body, "x:reader"); w.Code != 403 {
			t.Errorf("%s reader: %d", c.path, w.Code)
		}
	}
	if s.w.PropositionCount() != 0 || s.camara.Calls["fetch"] != 0 {
		t.Error("refused requests reached the source")
	}
}

func TestSearchAndImport(t *testing.T) {
	s := newServer()
	w := s.do("GET", "/api/team/sources/search?kind=PEC&number=221&year=2019", "", ed)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"external_id":"2192459"`) || !strings.Contains(w.Body.String(), `"linked_proposition_id":null`) {
		t.Fatalf("search: %d %s", w.Code, w.Body)
	}
	if w := s.do("GET", "/api/team/sources/search?kind=PEC&number=x&year=2019", "", ed); w.Code != 400 || code(t, w) != "invalid_query" {
		t.Errorf("bad number: %d %s", w.Code, w.Body)
	}

	w = s.do("POST", "/api/team/propositions/import", `{"chamber":"camara","external_id":"2192459"}`, ed)
	var res struct {
		PropositionID          string   `json:"proposition_id"`
		Created                bool     `json:"created"`
		SuggestionsUnavailable []string `json:"suggestions_unavailable"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &res) != nil || !res.Created || res.PropositionID == "" || res.SuggestionsUnavailable == nil {
		t.Fatalf("import: %d %s", w.Code, w.Body)
	}
	w = s.do("POST", "/api/team/propositions/import", `{"chamber":"camara","external_id":"2192459"}`, ed)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"created":false`) {
		t.Errorf("import again: %d %s", w.Code, w.Body)
	}
	if w := s.do("POST", "/api/team/propositions/import", `{"chamber":"alesp","external_id":"1"}`, ed); w.Code != 400 || code(t, w) != "unknown_chamber" {
		t.Errorf("unknown chamber: %d %s", w.Code, w.Body)
	}
	if w := s.do("POST", "/api/team/propositions/import", `{"chamber":"camara","external_id":"404"}`, ed); w.Code != 404 || code(t, w) != "source_record_not_found" {
		t.Errorf("not found: %d %s", w.Code, w.Body)
	}
}

func TestSourceDownIs503WithoutDetails(t *testing.T) {
	s := newServer()
	s.camara.FetchErr = fakes.Unavailable()
	w := s.do("POST", "/api/team/propositions/import", `{"chamber":"camara","external_id":"2192459"}`, ed)
	if w.Code != 503 || code(t, w) != "source_unavailable" || strings.Contains(w.Body.String(), "HTTP 503") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestRadarRoutes(t *testing.T) {
	s := newServer()
	agenda := time.Date(2026, 11, 12, 17, 0, 0, 0, time.UTC)
	s.camara.AgendaHits = []app.RadarHit{{Chamber: app.ChamberCamara, ExternalID: "77", Kind: "PL", Number: 10, Year: 2024,
		Summary: "Dispõe", SourceURL: "https://www.camara.leg.br/x", AgendaAt: &agenda}}
	if _, err := s.scan.Execute(t.Context()); err != nil {
		t.Fatal(err)
	}
	if w := s.do("GET", "/api/team/radar", "", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anônimo: %d", w.Code)
	}
	w := s.do("GET", "/api/team/radar", "", "u1:editor")
	var got struct {
		Items []struct {
			Chamber, Name string
			ExternalID    string  `json:"external_id"`
			AgendaAt      *string `json:"agenda_at"`
		}
		LastScanAt *string `json:"last_scan_at"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got.Items) != 1 || got.Items[0].Name != "PL 10/2024" ||
		got.Items[0].AgendaAt == nil || *got.Items[0].AgendaAt != "2026-11-12T17:00:00Z" || got.LastScanAt == nil {
		t.Fatalf("radar: %d %s", w.Code, w.Body)
	}
	if w := s.do("POST", "/api/team/radar/scan", "", "u1:editor"); w.Code != http.StatusAccepted || s.sched.Calls != 1 {
		t.Errorf("rodada: %d, %d", w.Code, s.sched.Calls)
	}
	if w := s.do("POST", "/api/team/radar/camara/77/dismiss", "", "u1:editor"); w.Code != http.StatusNoContent {
		t.Errorf("dispensar: %d %s", w.Code, w.Body)
	}
	if w := s.do("POST", "/api/team/radar/camara/77/dismiss", "", "u1:reader"); w.Code != http.StatusForbidden {
		t.Errorf("leitor: %d", w.Code)
	}
	if w := s.do("POST", "/api/team/radar/senado/1/dismiss", "", "u1:editor"); w.Code != http.StatusNotFound {
		t.Errorf("inexistente: %d", w.Code)
	}
	w = s.do("GET", "/api/team/radar", "", "u1:editor")
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Errorf("depois de dispensar: %s", w.Body)
	}
}
