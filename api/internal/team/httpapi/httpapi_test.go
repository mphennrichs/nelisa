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
	"github.com/mphennrichs/nelisa/api/internal/team/app"
	"github.com/mphennrichs/nelisa/api/internal/team/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
	"github.com/mphennrichs/nelisa/api/internal/team/httpapi"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

const bootstrapToken = "token-de-bootstrap-de-teste"

type server struct {
	t      *testing.T
	w      *fakes.World
	access auth.Tokens
	h      http.Handler
}

// newServer monta /api/team como o main monta, com fakes no lugar do banco.
func newServer(t *testing.T, secure bool, secret string) *server {
	t.Helper()
	w := fakes.NewWorld()
	clk := &clock.Fixed{T: t0}
	access := auth.NewTokens(strings.Repeat("s", 32), clk)
	sessions := app.NewSessions(access, w.Tokens(), clk)
	h := fakes.Hasher{}
	uc := httpapi.UseCases{
		Bootstrap:      app.NewBootstrap(w.Users(), h, sessions, w.Tx(), w.Audit(), clk, secret),
		Login:          app.NewLogin(w.Users(), h, sessions),
		Refresh:        app.NewRefresh(w.Tokens(), w.Users(), sessions, clk),
		Logout:         app.NewLogout(w.Tokens()),
		Authenticate:   app.NewAuthenticate(access, w.Users()),
		GetMe:          app.NewGetMe(w.Users()),
		ChangePassword: app.NewChangePassword(w.Users(), w.Tokens(), h, sessions, w.Tx(), clk),
		CreateUser:     app.NewCreateUser(w.Users(), h, w.Tx(), w.Audit(), clk),
		ResetPassword:  app.NewResetPassword(w.Users(), w.Tokens(), h, w.Tx(), w.Audit(), clk),
		UpdateRole:     app.NewUpdateRole(w.Users(), w.Tx(), w.Audit()),
		DeactivateUser: app.NewDeactivateUser(w.Users(), w.Tokens(), w.Tx(), w.Audit(), clk),
		ReactivateUser: app.NewReactivateUser(w.Users(), w.Tx(), w.Audit()),
		ListUsers:      app.NewListUsers(w.Users()),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := httpapi.New(uc, httpapi.Options{
		SecureCookies: secure,
		LimitKey:      func(r *http.Request) (string, error) { return r.RemoteAddr, nil },
	}, logger)

	r := chi.NewRouter()
	r.NotFound(httpx.NotFound)
	r.Route("/api/team", func(r chi.Router) {
		handlers.RegisterAuth(r)
		r.Group(func(r chi.Router) {
			r.Use(httpapi.Authenticator(uc.Authenticate, logger))
			handlers.RegisterAuthenticated(r)
		})
	})
	return &server{t: t, w: w, access: access, h: r}
}

func (s *server) seed(id, email, password string, role domain.Role) {
	s.t.Helper()
	u, err := domain.NewUser(id, "Pessoa "+id, email, "hashed:"+password, role, false, t0)
	if err != nil {
		s.t.Fatal(err)
	}
	s.w.Put(u)
}

func (s *server) tokenFor(id string, role auth.Role) string {
	tok, _, err := s.access.Issue(id, role)
	if err != nil {
		s.t.Fatal(err)
	}
	return tok
}

type req struct {
	method, path, body, bearer string
	cookie                     *http.Cookie
	remote                     string
}

func (s *server) do(r req) *httptest.ResponseRecorder {
	var body io.Reader
	if r.body != "" {
		body = strings.NewReader(r.body)
	}
	hr := httptest.NewRequest(r.method, r.path, body)
	if r.bearer != "" {
		hr.Header.Set("Authorization", "Bearer "+r.bearer)
	}
	if r.cookie != nil {
		hr.AddCookie(r.cookie)
	}
	if r.remote != "" {
		hr.RemoteAddr = r.remote
	}
	w := httptest.NewRecorder()
	s.h.ServeHTTP(w, hr)
	return w
}

func refreshCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == httpapi.RefreshCookieName {
			return c
		}
	}
	t.Fatalf("no refresh cookie in response (%d %s)", w.Code, w.Body)
	return nil
}

func wantError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var e httpx.ErrorBody
	if w.Code != status || json.Unmarshal(w.Body.Bytes(), &e) != nil || e.Error.Code != code || e.Error.Message == "" {
		t.Fatalf("got %d %s; want %d %q in the standard error format", w.Code, w.Body, status, code)
	}
}

type sessionBody struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresAt   string `json:"expires_at"`
	User        map[string]any
}

func decodeSession(t *testing.T, w *httptest.ResponseRecorder) sessionBody {
	t.Helper()
	var b sessionBody
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil || b.AccessToken == "" || b.TokenType != "Bearer" {
		t.Fatalf("session body: %s (%v)", w.Body, err)
	}
	return b
}

// --- login e cookie ---

func TestLoginSetsStrictHttpOnlyCookie(t *testing.T) {
	s := newServer(t, true, "")
	s.seed("u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)

	w := s.do(req{method: "POST", path: "/api/team/auth/login", body: `{"email":"ana@nelisa.org","password":"senha-certa"}`})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	b := decodeSession(t, w)
	if b.User["id"] != "u1" || b.User["role"] != "editor" {
		t.Errorf("user = %v", b.User)
	}
	if strings.Contains(w.Body.String(), "hashed:") || strings.Contains(w.Body.String(), "password_hash") {
		t.Fatal("password hash leaked in the response")
	}
	if b.ExpiresAt != t0.Add(auth.AccessTokenTTL).Format(time.RFC3339) {
		t.Errorf("expires_at = %s", b.ExpiresAt)
	}
	c := refreshCookie(t, w)
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/api/team/auth" || c.Value == "" || c.MaxAge <= 0 {
		t.Errorf("cookie attributes = %+v", c)
	}
	if strings.Contains(w.Body.String(), c.Value) {
		t.Error("the refresh token must only travel in the cookie")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("session responses must not be cached")
	}
}

func TestInsecureCookieOnlyWhenConfigured(t *testing.T) {
	s := newServer(t, false, "")
	s.seed("u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	w := s.do(req{method: "POST", path: "/api/team/auth/login", body: `{"email":"ana@nelisa.org","password":"senha-certa"}`})
	if c := refreshCookie(t, w); c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
		t.Errorf("dev cookie = %+v", c)
	}
}

func TestLoginErrors(t *testing.T) {
	s := newServer(t, true, "")
	s.seed("u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	wantError(t, s.do(req{method: "POST", path: "/api/team/auth/login", body: `{"email":"ana@nelisa.org","password":"errada"}`}),
		http.StatusUnauthorized, "invalid_credentials")
	wantError(t, s.do(req{method: "POST", path: "/api/team/auth/login", body: `{"email":"x@nelisa.org","password":"senha-certa"}`}),
		http.StatusUnauthorized, "invalid_credentials")
	wantError(t, s.do(req{method: "POST", path: "/api/team/auth/login", body: `{"email":"ana@nelisa.org","password":"senha-certa","role":"admin"}`}),
		http.StatusBadRequest, "invalid_json")
}

func TestLoginIsRateLimitedPerIP(t *testing.T) {
	s := newServer(t, true, "")
	body := `{"email":"x@nelisa.org","password":"errada-123"}`
	for i := range 10 {
		if w := s.do(req{method: "POST", path: "/api/team/auth/login", body: body, remote: "203.0.113.1:1000"}); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i+1, w.Code)
		}
	}
	wantError(t, s.do(req{method: "POST", path: "/api/team/auth/login", body: body, remote: "203.0.113.1:1000"}),
		http.StatusTooManyRequests, "too_many_requests")
	// another IP has its own bucket
	if w := s.do(req{method: "POST", path: "/api/team/auth/login", body: body, remote: "203.0.113.2:1000"}); w.Code != http.StatusUnauthorized {
		t.Errorf("other IP: %d", w.Code)
	}
}

// --- refresh e logout ---

func TestRefreshRotatesCookieAndDetectsReuse(t *testing.T) {
	s := newServer(t, true, "")
	s.seed("u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	login := s.do(req{method: "POST", path: "/api/team/auth/login", body: `{"email":"ana@nelisa.org","password":"senha-certa"}`})
	first := refreshCookie(t, login)

	w := s.do(req{method: "POST", path: "/api/team/auth/refresh", cookie: first})
	if w.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", w.Code, w.Body)
	}
	decodeSession(t, w)
	if second := refreshCookie(t, w); second.Value == first.Value {
		t.Fatal("refresh must rotate the cookie")
	}

	wantError(t, s.do(req{method: "POST", path: "/api/team/auth/refresh"}), http.StatusUnauthorized, "invalid_refresh_token")
	w = s.do(req{method: "POST", path: "/api/team/auth/refresh", cookie: &http.Cookie{Name: httpapi.RefreshCookieName, Value: "inventado"}})
	wantError(t, w, http.StatusUnauthorized, "invalid_refresh_token")
	if c := refreshCookie(t, w); c.MaxAge >= 0 {
		t.Errorf("a refused refresh must clear the cookie: %+v", c)
	}
}

func TestLogoutClearsCookieAndSession(t *testing.T) {
	s := newServer(t, true, "")
	s.seed("u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	c := refreshCookie(t, s.do(req{method: "POST", path: "/api/team/auth/login", body: `{"email":"ana@nelisa.org","password":"senha-certa"}`}))

	w := s.do(req{method: "POST", path: "/api/team/auth/logout", cookie: c})
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", w.Code)
	}
	if cleared := refreshCookie(t, w); cleared.MaxAge >= 0 || cleared.Value != "" || cleared.Path != "/api/team/auth" {
		t.Errorf("cookie not cleared: %+v", cleared)
	}
	wantError(t, s.do(req{method: "POST", path: "/api/team/auth/refresh", cookie: c}), http.StatusUnauthorized, "invalid_refresh_token")
	if w := s.do(req{method: "POST", path: "/api/team/auth/logout"}); w.Code != http.StatusNoContent {
		t.Errorf("logout without cookie is idempotent: %d", w.Code)
	}
}

// --- bootstrap ---

func TestBootstrapEndpoint(t *testing.T) {
	body := `{"token":"` + bootstrapToken + `","name":"Ana","email":"ana@nelisa.org","password":"senha-forte"}`

	wantError(t, newServer(t, true, "").do(req{method: "POST", path: "/api/team/auth/bootstrap", body: body}),
		http.StatusForbidden, "bootstrap_disabled")

	s := newServer(t, true, bootstrapToken)
	wantError(t, s.do(req{method: "POST", path: "/api/team/auth/bootstrap", body: strings.Replace(body, bootstrapToken, "errado", 1)}),
		http.StatusForbidden, "bootstrap_token_invalid")
	wantError(t, s.do(req{method: "POST", path: "/api/team/auth/bootstrap", body: strings.Replace(body, "senha-forte", "curta", 1)}),
		http.StatusBadRequest, "password_too_short")

	w := s.do(req{method: "POST", path: "/api/team/auth/bootstrap", body: body})
	if w.Code != http.StatusCreated {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body)
	}
	if b := decodeSession(t, w); b.User["role"] != "admin" {
		t.Errorf("first user must be admin: %v", b.User)
	}
	refreshCookie(t, w)
	wantError(t, s.do(req{method: "POST", path: "/api/team/auth/bootstrap", body: body}), http.StatusConflict, "already_bootstrapped")
}

// --- autenticação ---

func TestAuthenticatedRoutesRequireValidToken(t *testing.T) {
	s := newServer(t, true, "")
	s.seed("u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)

	for _, path := range []string{"/api/team/me", "/api/team/users"} {
		wantError(t, s.do(req{method: "GET", path: path}), http.StatusUnauthorized, "unauthenticated")
		wantError(t, s.do(req{method: "GET", path: path, bearer: "lixo"}), http.StatusUnauthorized, "unauthenticated")
		wantError(t, s.do(req{method: "GET", path: path, bearer: s.tokenFor("apagado", auth.RoleAdmin)}), http.StatusUnauthorized, "unauthenticated")
	}

	w := s.do(req{method: "GET", path: "/api/team/me", bearer: s.tokenFor("u1", auth.RoleEditor)})
	var me map[string]any
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &me) != nil || me["email"] != "ana@nelisa.org" {
		t.Fatalf("me: %d %s", w.Code, w.Body)
	}
	if _, leaked := me["password_hash"]; leaked {
		t.Fatal("password hash leaked")
	}
}

func TestEditorCannotReachUserManagement(t *testing.T) {
	s := newServer(t, true, "")
	s.seed("u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	s.seed("u2", "bia@nelisa.org", "senha-certa", domain.RoleReviewer)
	// a forged "admin" claim does not help: the role comes from the account
	for _, tok := range []string{s.tokenFor("u1", auth.RoleEditor), s.tokenFor("u1", auth.RoleAdmin), s.tokenFor("u2", auth.RoleReviewer)} {
		wantError(t, s.do(req{method: "GET", path: "/api/team/users", bearer: tok}), http.StatusForbidden, "forbidden")
		wantError(t, s.do(req{method: "PATCH", path: "/api/team/users/u1", bearer: tok, body: `{"role":"admin"}`}), http.StatusForbidden, "forbidden")
		wantError(t, s.do(req{method: "POST", path: "/api/team/users", bearer: tok,
			body: `{"name":"X","email":"x@nelisa.org","password":"senha-certa","role":"admin"}`}), http.StatusForbidden, "forbidden")
	}
	if len(s.w.Events()) != 0 {
		t.Error("refused calls were audited")
	}
}

// --- administração ---

func TestAdminManagesUsers(t *testing.T) {
	s := newServer(t, true, "")
	s.seed("admin", "admin@nelisa.org", "senha-certa", domain.RoleAdmin)
	tok := s.tokenFor("admin", auth.RoleAdmin)

	w := s.do(req{method: "POST", path: "/api/team/users", bearer: tok,
		body: `{"name":"Bia","email":"bia@nelisa.org","password":"temporaria","role":"editor"}`})
	var created map[string]any
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &created) != nil || created["must_change_password"] != true {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	id := created["id"].(string)

	wantError(t, s.do(req{method: "POST", path: "/api/team/users", bearer: tok,
		body: `{"name":"Outra","email":"BIA@nelisa.org","password":"temporaria","role":"editor"}`}), http.StatusConflict, "email_taken")
	wantError(t, s.do(req{method: "POST", path: "/api/team/users", bearer: tok,
		body: `{"name":"C","email":"c@nelisa.org","password":"temporaria","role":"root"}`}), http.StatusBadRequest, "invalid_role")
	wantError(t, s.do(req{method: "POST", path: "/api/team/users", bearer: tok,
		body: `{"name":"C","email":"c","password":"temporaria","role":"editor"}`}), http.StatusBadRequest, "invalid_email")

	w = s.do(req{method: "GET", path: "/api/team/users", bearer: tok})
	var list struct{ Users []map[string]any }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Users) != 2 {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}

	w = s.do(req{method: "PATCH", path: "/api/team/users/" + id, bearer: tok, body: `{"role":"reviewer"}`})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"role":"reviewer"`) {
		t.Fatalf("update role: %d %s", w.Code, w.Body)
	}
	wantError(t, s.do(req{method: "PATCH", path: "/api/team/users/admin", bearer: tok, body: `{"role":"editor"}`}), http.StatusConflict, "last_admin")
	wantError(t, s.do(req{method: "PATCH", path: "/api/team/users/nope", bearer: tok, body: `{"role":"editor"}`}), http.StatusNotFound, "user_not_found")

	if w := s.do(req{method: "POST", path: "/api/team/users/" + id + "/password", bearer: tok, body: `{"password":"outra-temporaria"}`}); w.Code != http.StatusNoContent {
		t.Fatalf("reset: %d %s", w.Code, w.Body)
	}
	wantError(t, s.do(req{method: "POST", path: "/api/team/users/" + id + "/password", bearer: tok, body: `{"password":"curta"}`}),
		http.StatusBadRequest, "password_too_short")

	wantError(t, s.do(req{method: "POST", path: "/api/team/users/admin/deactivate", bearer: tok}), http.StatusConflict, "cannot_deactivate_self")
	w = s.do(req{method: "POST", path: "/api/team/users/" + id + "/deactivate", bearer: tok})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"active":false`) || !strings.Contains(w.Body.String(), `"deactivated_at":"`) {
		t.Fatalf("deactivate: %d %s", w.Code, w.Body)
	}
	wantError(t, s.do(req{method: "POST", path: "/api/team/auth/login", body: `{"email":"bia@nelisa.org","password":"outra-temporaria"}`}),
		http.StatusForbidden, "account_deactivated")
	wantError(t, s.do(req{method: "POST", path: "/api/team/users/nope/deactivate", bearer: tok}), http.StatusNotFound, "user_not_found")
	if w := s.do(req{method: "DELETE", path: "/api/team/users/" + id, bearer: tok}); w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("DELETE must not exist anymore: %d", w.Code)
	}
	w = s.do(req{method: "POST", path: "/api/team/users/" + id + "/reactivate", bearer: tok})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"active":true`) || !strings.Contains(w.Body.String(), `"deactivated_at":null`) {
		t.Fatalf("reactivate: %d %s", w.Code, w.Body)
	}

	var actions []string
	for _, e := range s.w.Events() {
		actions = append(actions, e.Action)
	}
	want := []string{app.ActionUserCreate, app.ActionUserRoleChange, app.ActionUserPasswordReset, app.ActionUserDeactivate, app.ActionUserReactivate}
	if strings.Join(actions, ",") != strings.Join(want, ",") {
		t.Errorf("audit actions = %v, want %v", actions, want)
	}
}

// --- troca de senha ---

func TestChangePasswordEndpoint(t *testing.T) {
	s := newServer(t, true, "")
	s.seed("u1", "ana@nelisa.org", "senha-antiga", domain.RoleEditor)
	tok := s.tokenFor("u1", auth.RoleEditor)

	wantError(t, s.do(req{method: "POST", path: "/api/team/me/password", bearer: tok,
		body: `{"current_password":"errada","new_password":"senha-nova-1"}`}), http.StatusForbidden, "wrong_password")
	wantError(t, s.do(req{method: "POST", path: "/api/team/me/password", bearer: tok,
		body: `{"current_password":"senha-antiga","new_password":"curta"}`}), http.StatusBadRequest, "password_too_short")

	w := s.do(req{method: "POST", path: "/api/team/me/password", bearer: tok,
		body: `{"current_password":"senha-antiga","new_password":"senha-nova-1"}`})
	if w.Code != http.StatusOK {
		t.Fatalf("change: %d %s", w.Code, w.Body)
	}
	b := decodeSession(t, w)
	refreshCookie(t, w)
	if s.do(req{method: "GET", path: "/api/team/me", bearer: b.AccessToken}).Code != http.StatusOK {
		t.Error("the new access token must work")
	}
}
