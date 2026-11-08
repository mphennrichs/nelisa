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

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	"github.com/mphennrichs/nelisa/api/internal/proposition/httpapi"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

type server struct {
	w *fakes.World
	h http.Handler
}

// fakeAuth faz o papel do middleware do team: o header X-Test-Actor
// ("id:role") vira o Ator.
func fakeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-Test-Actor"); v != "" {
			id, role, _ := strings.Cut(v, ":")
			r = r.WithContext(auth.WithActor(r.Context(), auth.Actor{UserID: id, Role: auth.Role(role)}))
		}
		next.ServeHTTP(w, r)
	})
}

func newServer(t *testing.T) *server {
	t.Helper()
	w := fakes.NewWorld()
	clk := &clock.Fixed{T: t0}
	uc := httpapi.UseCases{
		ListPropositions: app.NewListPropositions(w.Props(), w.Links()),
		GetProposition:   app.NewGetProposition(w.Props(), w.Links(), w.Timeline()),
		ConfirmLink:      app.NewConfirmLink(w.Links(), w.Tx(), w.Audit(), w.Scheduler(), clk),
		RejectLink:       app.NewRejectLink(w.Links(), w.Tx(), w.Audit()),
		SubmitForReview:  app.NewSubmitForReview(w.Props(), w.Tx(), w.Audit(), clk),
		ListPublic:       app.NewListPublicPropositions(w.Props(), w.Links()),
		GetPublic:        app.NewGetPublicProposition(w.Props(), w.Links(), w.Timeline()),
	}
	h := httpapi.New(uc, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.NotFound(httpx.NotFound)
	r.Route("/api/public", h.RegisterPublic)
	r.Route("/api/team", func(r chi.Router) {
		r.Use(fakeAuth)
		h.RegisterTeam(r)
	})
	return &server{w: w, h: r}
}

func (s *server) do(method, path, actor string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if actor != "" {
		r.Header.Set("X-Test-Actor", actor)
	}
	w := httptest.NewRecorder()
	s.h.ServeHTTP(w, r)
	return w
}

// seed cria a PEC 221/2019 em state, com um Vínculo confirmado que tem um
// erro de sincronização e um sugerido: dados internos que nunca podem sair
// pela rota pública.
func (s *server) seed(t *testing.T, state domain.ReviewState) domain.Proposition {
	t.Helper()
	p, err := domain.NewProposition("11111111-1111-4111-8111-111111111111", "pec-221-2019", "PEC", 221, 2019,
		domain.ChamberCamara, domain.OfficialData{OfficialSummary: "Altera a jornada", Status: "Em tramitação"}, t0)
	if err != nil {
		t.Fatal(err)
	}
	p.ReviewState = state
	s.w.Put(p)
	l, _ := domain.NewConfirmedLink("l-camara", p.ID, domain.ChamberCamara, "2192459", "PEC 221/2019", "https://www.camara.leg.br/p", "segredo-editor-id", t0)
	l.SyncFailed("SEGREDO-ERRO-INTERNO")
	s.w.PutLink(l)
	sug, _ := domain.NewSuggestedLink("l-senado", p.ID, domain.ChamberSenado, "8441243", "PEC 221/2019", "https://www25.senado.leg.br/sugestao", t0)
	s.w.PutLink(sug)
	s.w.SetTimeline(p.ID, []app.TimelineEntry{{Chamber: "camara", At: t0, Body: "PLEN", Description: "Apresentação"}})
	return p
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var e httpx.ErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("not the standard error body: %s", w.Body)
	}
	return e.Error.Code
}

func TestPublicDraftIsNotFound(t *testing.T) {
	s := newServer(t)
	s.seed(t, domain.ReviewDraft)
	w := s.do("GET", "/api/public/propositions/pec-221-2019", "")
	if w.Code != http.StatusNotFound || errCode(t, w) != "proposition_not_found" {
		t.Errorf("draft by slug: %d %s", w.Code, w.Body)
	}
	w = s.do("GET", "/api/public/propositions", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "pec-221-2019") {
		t.Errorf("draft listed: %s", w.Body)
	}
	// Nem pelo id interno nem por slug inventado.
	for _, path := range []string{"/api/public/propositions/11111111-1111-4111-8111-111111111111", "/api/public/propositions/nao-existe"} {
		if w := s.do("GET", path, ""); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
}

func TestPublicNeverLeaksInternalFields(t *testing.T) {
	s := newServer(t)
	s.seed(t, domain.ReviewInReview)
	for _, path := range []string{"/api/public/propositions", "/api/public/propositions/pec-221-2019"} {
		w := s.do("GET", path, "")
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		body := w.Body.String()
		for _, leak := range []string{
			"SEGREDO-ERRO-INTERNO", "segredo-editor-id", "last_sync_error", "confirmed_by", "review_state",
			"senado.leg.br/sugestao", "l-camara", "11111111-1111-4111-8111-111111111111",
		} {
			if strings.Contains(body, leak) {
				t.Errorf("%s leaks %q: %s", path, leak, body)
			}
		}
		if !strings.Contains(body, `"under_review":true`) {
			t.Errorf("%s must flag under_review: %s", path, body)
		}
	}
	var d struct {
		Name     string
		Sources  []struct{ Chamber, URL string }
		Timeline []struct{ Description string }
	}
	w := s.do("GET", "/api/public/propositions/pec-221-2019", "")
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil || d.Name != "PEC 221/2019" || len(d.Sources) != 1 || len(d.Timeline) != 1 {
		t.Errorf("detail = %+v (%v)", d, err)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.HasPrefix(cc, "public") {
		t.Errorf("Cache-Control = %q", cc)
	}
}

func TestTeamRoutesAuthorization(t *testing.T) {
	s := newServer(t)
	p := s.seed(t, domain.ReviewDraft)
	paths := []struct{ method, path string }{
		{"GET", "/api/team/propositions"},
		{"GET", "/api/team/propositions/" + p.ID},
		{"POST", "/api/team/propositions/" + p.ID + "/submit-for-review"},
		{"POST", "/api/team/propositions/" + p.ID + "/links/l-senado/confirm"},
		{"POST", "/api/team/propositions/" + p.ID + "/links/l-senado/reject"},
	}
	for _, c := range paths {
		if w := s.do(c.method, c.path, ""); w.Code != 401 || errCode(t, w) != "unauthenticated" {
			t.Errorf("%s %s anonymous: %d", c.method, c.path, w.Code)
		}
		if w := s.do(c.method, c.path, "x:reader"); w.Code != 403 {
			t.Errorf("%s %s reader: %d", c.method, c.path, w.Code)
		}
	}
	if len(s.w.Events()) != 0 {
		t.Error("refused requests audited something")
	}
}

func TestTeamFlow(t *testing.T) {
	s := newServer(t)
	p := s.seed(t, domain.ReviewDraft)
	ed := "editor-1:editor"

	w := s.do("GET", "/api/team/propositions/"+p.ID, ed)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"review_state":"draft"`) || !strings.Contains(w.Body.String(), "SEGREDO-ERRO-INTERNO") {
		t.Fatalf("team sees everything: %d %s", w.Code, w.Body)
	}
	if w := s.do("GET", "/api/team/propositions/nao-existe", ed); w.Code != 404 {
		t.Errorf("unknown: %d", w.Code)
	}

	w = s.do("POST", "/api/team/propositions/"+p.ID+"/links/l-senado/confirm", ed)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"confirmed"`) {
		t.Fatalf("confirm: %d %s", w.Code, w.Body)
	}
	if w := s.do("POST", "/api/team/propositions/"+p.ID+"/links/l-senado/confirm", ed); w.Code != 409 || errCode(t, w) != "link_not_suggested" {
		t.Errorf("confirm twice: %d %s", w.Code, w.Body)
	}
	if w := s.do("POST", "/api/team/propositions/"+p.ID+"/links/l-camara/reject", ed); w.Code != 409 {
		t.Errorf("reject confirmed: %d", w.Code)
	}

	w = s.do("POST", "/api/team/propositions/"+p.ID+"/submit-for-review", ed)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "in_review") {
		t.Fatalf("submit: %d %s", w.Code, w.Body)
	}
	if w := s.do("POST", "/api/team/propositions/"+p.ID+"/submit-for-review", ed); w.Code != 409 || errCode(t, w) != "invalid_review_transition" {
		t.Errorf("submit twice: %d %s", w.Code, w.Body)
	}
	if w := s.do("GET", "/api/public/propositions/pec-221-2019", ""); w.Code != 200 {
		t.Errorf("in_review must be public: %d", w.Code)
	}
	if got := len(s.w.Events()); got != 2 {
		t.Errorf("events = %d, want 2", got)
	}

	w = s.do("GET", "/api/team/propositions", ed)
	var list struct {
		Propositions []struct {
			ReviewState string `json:"review_state"`
			Links       []struct{ State string }
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Propositions) != 1 || len(list.Propositions[0].Links) != 2 {
		t.Errorf("list: %v %s", err, w.Body)
	}
}
