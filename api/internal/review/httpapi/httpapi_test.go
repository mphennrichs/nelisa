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
	"github.com/mphennrichs/nelisa/api/internal/review/app"
	"github.com/mphennrichs/nelisa/api/internal/review/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
	"github.com/mphennrichs/nelisa/api/internal/review/httpapi"
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

func newServer(w *fakes.World) http.Handler {
	clk := &clock.Fixed{T: t0}
	h := httpapi.New(httpapi.UseCases{
		Approve:      app.NewApprove(w.Approvals(), w.Editorial(), w.Claims(), w.Tx(), w.Audit(), clk),
		SecondReview: app.NewSecondReview(w.Approvals(), w.Editorial(), w.Tx(), w.Audit(), clk),
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.Route("/api/team", func(r chi.Router) {
		r.Use(fakeAuth)
		h.RegisterTeam(r)
	})
	return r
}

func do(h http.Handler, path, actor string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, nil)
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

const approve = "/api/team/propositions/p1/versions/v1/approve"
const second = "/api/team/propositions/p1/versions/v1/second-review"

func world(authors []string, missing ...domain.Blocker) *fakes.World {
	w := fakes.NewWorld()
	w.PutProposition(fakes.Proposition{ID: "p1", Name: "PEC 1/2020", State: "in_review"})
	w.PutVersion(fakes.Version{ID: "v1", PropositionID: "p1", Seq: 1, State: "pending", Authors: authors, Missing: missing})
	return w
}

func TestApproveRoutes(t *testing.T) {
	h := newServer(world([]string{"ana", "rev"}))
	if w := do(h, approve, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous: %d", w.Code)
	}
	if w := do(h, approve, "ana:editor"); w.Code != http.StatusForbidden || errCode(t, w) != "forbidden" {
		t.Errorf("editor: %d %s", w.Code, w.Body)
	}
	if w := do(h, approve, "rev:reviewer"); w.Code != http.StatusForbidden || errCode(t, w) != "four_eyes" {
		t.Errorf("author: %d %s", w.Code, w.Body)
	}
	if w := do(h, "/api/team/propositions/p1/versions/nope/approve", "rev2:reviewer"); w.Code != 404 || errCode(t, w) != "version_not_found" {
		t.Errorf("unknown version: %d %s", w.Code, w.Body)
	}
	w := do(h, approve, "rev2:reviewer")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"four_eyes":true`) || !strings.Contains(w.Body.String(), `"review_badge":2`) {
		t.Fatalf("approve: %d %s", w.Code, w.Body)
	}
	if w := do(h, approve, "rev2:reviewer"); w.Code != 409 || errCode(t, w) != "version_not_pending" {
		t.Errorf("twice: %d %s", w.Code, w.Body)
	}
	if w := do(h, second, "rev3:reviewer"); w.Code != 409 || errCode(t, w) != "no_second_review_pending" {
		t.Errorf("second on four eyes: %d %s", w.Code, w.Body)
	}
}

func TestApproveIncompleteContent(t *testing.T) {
	h := newServer(world([]string{"ana"}, domain.Blocker{Code: "needs_confirmation", Message: "Confirme o conteúdo trazido."}))
	w := do(h, approve, "rev:reviewer")
	if w.Code != 409 || errCode(t, w) != "content_incomplete" || !strings.Contains(w.Body.String(), "Confirme o conteúdo trazido.") {
		t.Errorf("incomplete: %d %s", w.Code, w.Body)
	}
}

func TestAdminSelfApprovalAndSecondReviewRoutes(t *testing.T) {
	h := newServer(world([]string{"adm"}))
	w := do(h, approve, "adm:admin")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"four_eyes":false`) || !strings.Contains(w.Body.String(), `"review_badge":1`) ||
		!strings.Contains(w.Body.String(), `"pending_second_review":true`) {
		t.Fatalf("admin self approval: %d %s", w.Code, w.Body)
	}
	if w := do(h, second, "adm:admin"); w.Code != 403 || errCode(t, w) != "four_eyes" {
		t.Errorf("approver second review: %d %s", w.Code, w.Body)
	}
	if w := do(h, second, "ana:editor"); w.Code != 403 {
		t.Errorf("editor second review: %d", w.Code)
	}
	w = do(h, second, "rev:reviewer")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"review_badge":2`) {
		t.Errorf("second review: %d %s", w.Code, w.Body)
	}
}
