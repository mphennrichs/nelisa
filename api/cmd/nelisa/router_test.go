package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	auditapp "github.com/mphennrichs/nelisa/api/internal/audit/app"
	ingestapp "github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/config"
	"github.com/mphennrichs/nelisa/api/internal/platform/httpx"
	progressfakes "github.com/mphennrichs/nelisa/api/internal/progress/app/fakes"
	propfakes "github.com/mphennrichs/nelisa/api/internal/proposition/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/team/app/fakes"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

// testConfig é uma config mínima válida para montar o roteador.
func testConfig() config.Config {
	return config.Config{JWTSecret: strings.Repeat("s", 32), SecureCookies: true}
}

func fakeTeam(logger *slog.Logger) teamModule {
	w := fakes.NewWorld()
	p := teamPorts{users: w.Users(), tokens: w.Tokens(), tx: w.Tx(), audit: w.Audit(), hasher: fakes.Hasher{}}
	return newTeamModule(testConfig(), p, clock.System{}, logger)
}

// noScheduler descarta os jobs: o roteador não precisa do River.
type noScheduler struct{}

func (noScheduler) ScheduleSync(context.Context, string) error    { return nil }
func (noScheduler) ScheduleSyncs(context.Context, []string) error { return nil }

func fakeContent(logger *slog.Logger) contentModule {
	w := propfakes.NewWorld()
	p := contentPorts{
		props: w.Props(), links: w.Links(), entries: progressfakes.NewEntries(), tx: w.Tx(),
		audit:     func(context.Context, auditapp.RecordEventInput) error { return nil },
		scheduler: noScheduler{}, sources: ingestapp.NewSources(),
	}
	return newContentModule(newContentUseCases(p, clock.System{}), logger)
}

func serve(p Pinger, method, path string) *httptest.ResponseRecorder {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := newRouter(testConfig(), p, fakeTeam(logger), fakeContent(logger), logger)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestHealthz(t *testing.T) {
	if w := serve(fakePinger{}, "GET", "/healthz"); w.Code != http.StatusOK {
		t.Errorf("healthy db: status %d", w.Code)
	}
	if w := serve(fakePinger{errors.New("down")}, "GET", "/healthz"); w.Code != http.StatusServiceUnavailable {
		t.Errorf("db down: status %d", w.Code)
	}
}

func TestVersion(t *testing.T) {
	w := serve(fakePinger{}, "GET", "/version")
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["commit"] == "" {
		t.Errorf("unexpected /version body: %s", w.Body)
	}
}

func TestUnknownRouteUsesStandardError(t *testing.T) {
	w := serve(fakePinger{}, "GET", "/api/public/nope")
	var e httpx.ErrorBody
	if w.Code != http.StatusNotFound || json.Unmarshal(w.Body.Bytes(), &e) != nil || e.Error.Code != "not_found" {
		t.Errorf("got %d %s", w.Code, w.Body)
	}
}

func TestTeamRoutesAreMounted(t *testing.T) {
	// /auth é pública (aqui, credencial errada → 401 invalid_credentials);
	// o resto de /api/team exige token; rota inexistente é 404.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := newRouter(testConfig(), fakePinger{}, fakeTeam(logger), fakeContent(logger), logger)
	cases := []struct {
		method, path, body string
		status             int
		code               string
	}{
		{"POST", "/api/team/auth/login", `{"email":"a@b.org","password":"xxxxxxxx"}`, 401, "invalid_credentials"},
		{"GET", "/api/team/me", "", 401, "unauthenticated"},
		{"GET", "/api/team/users", "", 401, "unauthenticated"},
		{"GET", "/api/team/nope", "", 404, "not_found"},
		{"GET", "/api/team/propositions", "", 401, "unauthenticated"},
		{"POST", "/api/team/propositions/import", `{}`, 401, "unauthenticated"},
		{"GET", "/api/team/sources/search?kind=PEC&number=1&year=2020", "", 401, "unauthenticated"},
		{"GET", "/api/public/propositions/pec-1-2020", "", 404, "proposition_not_found"},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		r.RemoteAddr = "192.0.2.1:1234"
		h.ServeHTTP(w, r)
		var e httpx.ErrorBody
		if w.Code != c.status || json.Unmarshal(w.Body.Bytes(), &e) != nil || e.Error.Code != c.code {
			t.Errorf("%s %s: got %d %s", c.method, c.path, w.Code, w.Body)
		}
	}
}
