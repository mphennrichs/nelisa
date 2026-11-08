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
	"github.com/mphennrichs/nelisa/api/internal/queue/app"
	"github.com/mphennrichs/nelisa/api/internal/queue/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
	"github.com/mphennrichs/nelisa/api/internal/queue/httpapi"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func fakeAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-Test-Actor"); v != "" {
			id, role, _ := strings.Cut(v, ":")
			r = r.WithContext(auth.WithActor(r.Context(), auth.Actor{UserID: id, Role: auth.Role(role)}))
		}
		next.ServeHTTP(w, r)
	})
}

func newServer() (*fakes.World, http.Handler) {
	w := fakes.NewWorld()
	w.People = map[string]app.Person{"ana": {Name: "Ana", Active: true}, "bia": {Name: "Bia", Active: true}, "rev": {Name: "Rui", Active: true}}
	w.Described["p1"] = app.Proposition{ID: "p1", Name: "PEC 1/2020", Slug: "pec-1-2020", Sphere: domain.SphereFederal, Ready: true,
		Current: &app.Version{ID: "v1", Seq: 1, Label: "Texto original", CreatedAt: t0}}
	w.InReview = []app.Proposition{{ID: "p2", Name: "PL 2/2026", Slug: "pl-2-2026", Sphere: domain.SphereState, InReview: true,
		Pending: &app.Version{ID: "v2", Seq: 1, Label: "Texto original", CreatedAt: t0}, Missing: []app.Missing{{Code: "summary", Message: "Escreva o Resumo."}}}}
	w.SecondReviews = []app.PendingSecondReview{{PropositionID: "p1", VersionID: "v1", ApprovedAt: t0}}
	clk := &clock.Fixed{T: t0}
	h := httpapi.New(httpapi.UseCases{
		Claim:     app.NewClaim(w.Claims(), w.Props(), w.Directory(), w.Tx(), clk),
		Release:   app.NewRelease(w.Claims(), w.Directory(), w.Tx(), w.Audit(), clk),
		ListQueue: app.NewListQueue(w.Props(), w.Reviews(), w.Claims(), w.Directory(), clk),
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.Route("/api/team", func(r chi.Router) {
		r.Use(fakeAuth)
		h.RegisterTeam(r)
	})
	return w, r
}

func do(h http.Handler, method, path, actor string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if actor != "" {
		r.Header.Set("X-Test-Actor", actor)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var e httpx.ErrorBody
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatalf("not the standard error body: %s", w.Body)
	}
	return e.Error.Code
}

func TestClaimRoutes(t *testing.T) {
	_, h := newServer()
	if w := do(h, "POST", "/api/team/propositions/p1/claim", ""); w.Code != 401 {
		t.Errorf("anonymous: %d", w.Code)
	}
	w := do(h, "POST", "/api/team/propositions/p1/claim", "ana:editor")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"holder_name":"Ana"`) || !strings.Contains(w.Body.String(), `"expires_at":"2026-11-11T12:00:00Z"`) {
		t.Fatalf("claim: %d %s", w.Code, w.Body)
	}
	w = do(h, "POST", "/api/team/propositions/p1/claim", "bia:editor")
	if w.Code != 409 || errCode(t, w) != "claim_taken" || !strings.Contains(w.Body.String(), "Ana está com a posse") {
		t.Errorf("taken: %d %s", w.Code, w.Body)
	}
	if w := do(h, "DELETE", "/api/team/propositions/p1/claim", "bia:editor"); w.Code != 403 {
		t.Errorf("editor releases other's: %d %s", w.Code, w.Body)
	}
	if w := do(h, "DELETE", "/api/team/propositions/p1/claim", "rev:reviewer"); w.Code != 204 {
		t.Errorf("reviewer release: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/api/team/propositions/nope/claim", "ana:editor"); w.Code != 404 || errCode(t, w) != "proposition_not_found" {
		t.Errorf("unknown: %d %s", w.Code, w.Body)
	}
}

func TestQueueRoute(t *testing.T) {
	_, h := newServer()
	w := do(h, "GET", "/api/team/queue", "ana:editor")
	body := w.Body.String()
	if w.Code != 200 || strings.Index(body, `"proposition_id":"p1"`) > strings.Index(body, `"proposition_id":"p2"`) {
		t.Fatalf("federal first: %d %s", w.Code, body)
	}
	for _, want := range []string{`"reason":"second_review"`, `"reason":"review"`, `"missing":[{"code":"summary"`, `"claim":null`, `"version":{"id":"v1","seq":1,"label":"Texto original"}`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
	w = do(h, "GET", "/api/team/queue?sphere=state", "ana:editor")
	if strings.Contains(w.Body.String(), `"p1"`) {
		t.Errorf("filter: %s", w.Body)
	}
	if w := do(h, "GET", "/api/team/queue?sphere=x", "ana:editor"); w.Code != 400 || errCode(t, w) != "invalid_sphere" {
		t.Errorf("bad sphere: %d %s", w.Code, w.Body)
	}
}
