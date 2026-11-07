package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	auditstore "github.com/mphennrichs/nelisa/api/internal/audit/store"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
	teamapp "github.com/mphennrichs/nelisa/api/internal/team/app"
)

// Teste de ponta a ponta com o Postgres real: roteador, casos de uso, stores
// do team e da Auditoria ligados como em produção.
func TestTeamEndToEndWithAudit(t *testing.T) {
	pool, _ := testdb.New(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := testConfig()
	cfg.BootstrapToken = "segredo-de-bootstrap-e2e"
	clk := clock.System{}
	h := newRouter(cfg, pool, newTeamModule(cfg, postgresTeamPorts(pool, clk), clk, logger), logger)

	do := func(method, path, body, bearer string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "192.0.2.10:1234"
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	var session struct {
		AccessToken string `json:"access_token"`
		User        struct{ ID, Role string }
	}

	w := do("POST", "/api/team/auth/bootstrap",
		`{"token":"segredo-de-bootstrap-e2e","name":"Ana","email":"ana@nelisa.org","password":"senha-forte"}`, "", nil)
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &session) != nil || session.User.Role != "admin" {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body)
	}
	adminID, adminTok := session.User.ID, session.AccessToken

	w = do("POST", "/api/team/users", `{"name":"Bia","email":"bia@nelisa.org","password":"temporaria","role":"editor"}`, adminTok, nil)
	var created struct{ ID string }
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &created) != nil {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}

	w = do("POST", "/api/team/auth/login", `{"email":"BIA@nelisa.org","password":"temporaria"}`, "", nil)
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &session) != nil {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	editorTok := session.AccessToken
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "nelisa_refresh" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no refresh cookie")
	}
	if w := do("PATCH", "/api/team/users/"+created.ID, `{"role":"admin"}`, editorTok, nil); w.Code != http.StatusForbidden {
		t.Fatalf("editor promoting herself: %d %s", w.Code, w.Body)
	}
	if w := do("POST", "/api/team/auth/refresh", "", "", cookie); w.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", w.Code, w.Body)
	}

	if w := do("PATCH", "/api/team/users/"+created.ID, `{"role":"reviewer"}`, adminTok, nil); w.Code != http.StatusOK {
		t.Fatalf("role: %d %s", w.Code, w.Body)
	}
	if w := do("POST", "/api/team/users/"+created.ID+"/password", `{"password":"nova-temporaria"}`, adminTok, nil); w.Code != http.StatusNoContent {
		t.Fatalf("reset: %d %s", w.Code, w.Body)
	}
	// the reset ended Bia's sessions
	if w := do("GET", "/api/team/me", "", editorTok, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("token from before the reset: %d %s", w.Code, w.Body)
	}
	if w := do("DELETE", "/api/team/users/"+created.ID, "", adminTok, nil); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}

	events, err := auditstore.New(pool).ListByTarget(context.Background(), "user", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range events {
		if e.ActorID != adminID {
			t.Errorf("event %s by %q, want admin", e.Action, e.ActorID)
		}
		actions = append(actions, e.Action)
	}
	want := []string{teamapp.ActionUserCreate, teamapp.ActionUserRoleChange, teamapp.ActionUserPasswordReset, teamapp.ActionUserDelete}
	if strings.Join(actions, ",") != strings.Join(want, ",") {
		t.Errorf("audit = %v, want %v", actions, want)
	}
	if boot, _ := auditstore.New(pool).ListByTarget(context.Background(), "user", adminID); len(boot) != 1 || boot[0].Action != teamapp.ActionUserBootstrap {
		t.Errorf("bootstrap audit = %+v", boot)
	}
	assertNoSecretsInAudit(t, pool)
}

func assertNoSecretsInAudit(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var dump string
	if err := pool.QueryRow(context.Background(), "SELECT coalesce(string_agg(metadata::text, ' '), '') FROM audit.events").Scan(&dump); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"temporaria", "senha-forte", "$2a$"} {
		if strings.Contains(dump, secret) {
			t.Fatalf("audit metadata carries a secret (%q): %s", secret, dump)
		}
	}
}
