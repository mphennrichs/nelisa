package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/team/app"
	"github.com/mphennrichs/nelisa/api/internal/team/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

const bootstrapToken = "token-de-bootstrap-de-teste"

type env struct {
	w        *fakes.World
	clk      *clock.Fixed
	access   auth.Tokens
	sessions *app.Sessions
}

func newEnv() *env {
	w := fakes.NewWorld()
	clk := &clock.Fixed{T: t0}
	access := auth.NewTokens(strings.Repeat("s", 32), clk)
	return &env{w: w, clk: clk, access: access, sessions: app.NewSessions(access, w.Tokens(), clk)}
}

// user cadastra direto no fake e devolve o usuário.
func (e *env) user(t *testing.T, id, email, password string, role domain.Role) domain.User {
	t.Helper()
	u, err := domain.NewUser(id, "Pessoa "+id, email, "hashed:"+password, role, false, e.clk.Now())
	if err != nil {
		t.Fatal(err)
	}
	e.w.Put(u)
	return u
}

func as(id string, role domain.Role) context.Context {
	return auth.WithActor(context.Background(), auth.Actor{UserID: id, Role: auth.Role(role)})
}

func (e *env) bootstrap(secret string) *app.Bootstrap {
	return app.NewBootstrap(e.w.Users(), fakes.Hasher{}, e.sessions, e.w.Tx(), e.w.Audit(), e.clk, secret)
}
func (e *env) login() *app.Login { return app.NewLogin(e.w.Users(), fakes.Hasher{}, e.sessions) }
func (e *env) refresh() *app.Refresh {
	return app.NewRefresh(e.w.Tokens(), e.w.Users(), e.sessions, e.clk)
}
func (e *env) createUser() *app.CreateUser {
	return app.NewCreateUser(e.w.Users(), fakes.Hasher{}, e.w.Tx(), e.w.Audit(), e.clk)
}
func (e *env) resetPassword() *app.ResetPassword {
	return app.NewResetPassword(e.w.Users(), e.w.Tokens(), fakes.Hasher{}, e.w.Tx(), e.w.Audit(), e.clk)
}
func (e *env) changePassword() *app.ChangePassword {
	return app.NewChangePassword(e.w.Users(), e.w.Tokens(), fakes.Hasher{}, e.sessions, e.w.Tx(), e.clk)
}
func (e *env) updateRole() *app.UpdateRole {
	return app.NewUpdateRole(e.w.Users(), e.w.Tx(), e.w.Audit())
}
func (e *env) deleteUser() *app.DeleteUser {
	return app.NewDeleteUser(e.w.Users(), e.w.Tx(), e.w.Audit())
}
func (e *env) authenticate() *app.Authenticate {
	return app.NewAuthenticate(e.access, e.w.Users())
}

// --- roles ---

func TestDomainRolesMatchAuthRoles(t *testing.T) {
	// team/domain é a fonte das roles; platform/auth precisa concordar
	// nos valores e na ordem, senão o middleware decide errado.
	roles := domain.Roles()
	for i, r := range roles {
		for j, min := range roles {
			if got := auth.Role(r).AtLeast(auth.Role(min)); got != (i >= j) {
				t.Errorf("auth.Role(%q).AtLeast(%q) = %v", r, min, got)
			}
		}
	}
}

// --- bootstrap ---

func TestBootstrapCreatesFirstAdminWithAudit(t *testing.T) {
	e := newEnv()
	s, err := e.bootstrap(bootstrapToken).Execute(context.Background(), app.BootstrapInput{
		Token: bootstrapToken, Name: "Ana", Email: "ana@nelisa.org", Password: "senha-forte",
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.User.Role != domain.RoleAdmin || s.User.MustChangePassword || s.AccessToken == "" || s.RefreshToken == "" {
		t.Fatalf("session = %+v", s)
	}
	if e.w.LockCalls != 1 {
		t.Errorf("bootstrap must lock users, LockCalls = %d", e.w.LockCalls)
	}
	ev := e.w.Events()
	if len(ev) != 1 || ev[0].Action != app.ActionUserBootstrap || ev[0].TargetID != s.User.ID || !ev[0].InTx {
		t.Fatalf("events = %+v", ev)
	}
}

func TestBootstrapRefusals(t *testing.T) {
	in := app.BootstrapInput{Token: bootstrapToken, Name: "Ana", Email: "ana@nelisa.org", Password: "senha-forte"}
	cases := []struct {
		name   string
		secret string
		mutate func(*app.BootstrapInput)
		seed   bool
		want   error
	}{
		{"disabled without BOOTSTRAP_TOKEN", "", nil, false, app.ErrBootstrapDisabled},
		{"wrong token", bootstrapToken, func(i *app.BootstrapInput) { i.Token = "errado" }, false, app.ErrBootstrapTokenInvalid},
		{"wrong token even when users exist", bootstrapToken, func(i *app.BootstrapInput) { i.Token = "" }, true, app.ErrBootstrapTokenInvalid},
		{"users already exist", bootstrapToken, nil, true, app.ErrAlreadyBootstrapped},
		{"short password", bootstrapToken, func(i *app.BootstrapInput) { i.Password = "1234567" }, false, domain.ErrPasswordTooShort},
		{"bad email", bootstrapToken, func(i *app.BootstrapInput) { i.Email = "ana" }, false, domain.ErrInvalidEmail},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv()
			if c.seed {
				e.user(t, "u1", "x@nelisa.org", "qualquer-senha", domain.RoleEditor)
			}
			input := in
			if c.mutate != nil {
				c.mutate(&input)
			}
			if _, err := e.bootstrap(c.secret).Execute(context.Background(), input); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if len(e.w.Events()) != 0 {
				t.Error("a refused bootstrap must not be audited")
			}
		})
	}
}

// --- login ---

func TestLogin(t *testing.T) {
	e := newEnv()
	u := e.user(t, "u1", "Ana@Nelisa.org", "senha-certa", domain.RoleEditor)

	s, err := e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-certa"})
	if err != nil {
		t.Fatalf("email is case-insensitive: %v", err)
	}
	if s.User.ID != u.ID || len(e.w.TokensOf(u.ID)) != 1 {
		t.Fatalf("session = %+v", s)
	}
	c, err := e.access.Verify(s.AccessToken)
	if err != nil || c.UserID != u.ID || c.Role != auth.RoleEditor {
		t.Fatalf("access token claims = %+v, %v", c, err)
	}

	for name, in := range map[string]app.LoginInput{
		"wrong password": {Email: "ana@nelisa.org", Password: "senha-errada"},
		"unknown email":  {Email: "ninguem@nelisa.org", Password: "senha-certa"},
	} {
		if _, err := e.login().Execute(context.Background(), in); !errors.Is(err, app.ErrInvalidCredentials) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

// --- refresh ---

func TestRefreshRotatesToken(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	s, _ := e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-certa"})

	e.clk.Advance(time.Minute)
	s2, err := e.refresh().Execute(context.Background(), app.RefreshInput{RefreshToken: s.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	if s2.RefreshToken == s.RefreshToken {
		t.Fatal("refresh must rotate the token")
	}
	// within grace, the same old token still works (parallel refreshes)
	e.clk.Advance(domain.ReuseGrace - time.Second)
	if _, err := e.refresh().Execute(context.Background(), app.RefreshInput{RefreshToken: s.RefreshToken}); err != nil {
		t.Fatalf("parallel refresh within grace: %v", err)
	}
}

func TestRefreshReuseAfterGraceEndsAllSessions(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	s, _ := e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-certa"})
	s2, err := e.refresh().Execute(context.Background(), app.RefreshInput{RefreshToken: s.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}

	e.clk.Advance(domain.ReuseGrace)
	if _, err := e.refresh().Execute(context.Background(), app.RefreshInput{RefreshToken: s.RefreshToken}); !errors.Is(err, domain.ErrRefreshTokenReused) {
		t.Fatalf("reuse: err = %v", err)
	}
	if n := len(e.w.TokensOf("u1")); n != 0 {
		t.Fatalf("reuse must end every session, %d left", n)
	}
	if _, err := e.refresh().Execute(context.Background(), app.RefreshInput{RefreshToken: s2.RefreshToken}); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Fatalf("the legitimate new token dies too: err = %v", err)
	}
}

func TestRefreshRefusals(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	s, _ := e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-certa"})

	if _, err := e.refresh().Execute(context.Background(), app.RefreshInput{RefreshToken: "inventado"}); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Errorf("unknown: %v", err)
	}
	if _, err := e.refresh().Execute(context.Background(), app.RefreshInput{}); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Errorf("empty: %v", err)
	}
	e.clk.Advance(domain.RefreshTokenTTL)
	if _, err := e.refresh().Execute(context.Background(), app.RefreshInput{RefreshToken: s.RefreshToken}); !errors.Is(err, domain.ErrRefreshTokenExpired) {
		t.Errorf("expired: %v", err)
	}
}

func TestRefreshPicksUpNewRole(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	s, _ := e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-certa"})
	_, _ = e.w.Users().UpdateRole(context.Background(), "u1", domain.RoleReviewer)
	s2, err := e.refresh().Execute(context.Background(), app.RefreshInput{RefreshToken: s.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := e.access.Verify(s2.AccessToken); c.Role != auth.RoleReviewer {
		t.Errorf("role = %q", c.Role)
	}
}

// --- logout ---

func TestLogoutDeletesSessionAndIsIdempotent(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	s, _ := e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-certa"})
	logout := app.NewLogout(e.w.Tokens())
	for range 2 {
		if err := logout.Execute(context.Background(), app.LogoutInput{RefreshToken: s.RefreshToken}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.refresh().Execute(context.Background(), app.RefreshInput{RefreshToken: s.RefreshToken}); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Errorf("logged-out token must not refresh: %v", err)
	}
}

// --- authenticate ---

func TestAuthenticate(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	tok, _, _ := e.access.Issue("u1", auth.RoleAdmin) // claim says admin...

	a, err := e.authenticate().Execute(context.Background(), app.AuthenticateInput{AccessToken: tok})
	if err != nil {
		t.Fatal(err)
	}
	if a.Role != auth.RoleEditor {
		t.Errorf("role must come from the account, not the token: %q", a.Role)
	}

	if _, err := e.authenticate().Execute(context.Background(), app.AuthenticateInput{AccessToken: "lixo"}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("garbage: %v", err)
	}
	ghost, _, _ := e.access.Issue("deleted", auth.RoleAdmin)
	if _, err := e.authenticate().Execute(context.Background(), app.AuthenticateInput{AccessToken: ghost}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("deleted user: %v", err)
	}

	e.clk.Advance(time.Second)
	if err := e.resetPassword().Execute(as("admin", domain.RoleAdmin), app.ResetPasswordInput{UserID: "u1", NewPassword: "nova-senha-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.authenticate().Execute(context.Background(), app.AuthenticateInput{AccessToken: tok}); !errors.Is(err, app.ErrSessionExpired) {
		t.Errorf("token from before a reset: %v", err)
	}
}

// --- get me ---

func TestGetMe(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	me := app.NewGetMe(e.w.Users())
	u, err := me.Execute(as("u1", domain.RoleEditor), app.GetMeInput{})
	if err != nil || u.ID != "u1" {
		t.Fatalf("u = %+v, %v", u, err)
	}
	if _, err := me.Execute(context.Background(), app.GetMeInput{}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("no actor: %v", err)
	}
}

// --- change password ---

func TestChangePassword(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-antiga", domain.RoleEditor)
	old, _ := e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-antiga"})
	ctx := as("u1", domain.RoleEditor)

	if _, err := e.changePassword().Execute(ctx, app.ChangePasswordInput{CurrentPassword: "errada", NewPassword: "senha-nova-1"}); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Errorf("wrong current: %v", err)
	}
	if _, err := e.changePassword().Execute(ctx, app.ChangePasswordInput{CurrentPassword: "senha-antiga", NewPassword: "curta"}); !errors.Is(err, domain.ErrPasswordTooShort) {
		t.Errorf("short new: %v", err)
	}
	if _, err := e.changePassword().Execute(context.Background(), app.ChangePasswordInput{}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("no actor: %v", err)
	}

	e.clk.Advance(time.Second)
	s, err := e.changePassword().Execute(ctx, app.ChangePasswordInput{CurrentPassword: "senha-antiga", NewPassword: "senha-nova-1"})
	if err != nil {
		t.Fatal(err)
	}
	// the new session works, the old ones are gone
	if _, err := e.authenticate().Execute(context.Background(), app.AuthenticateInput{AccessToken: s.AccessToken}); err != nil {
		t.Errorf("new access token refused: %v", err)
	}
	if _, err := e.authenticate().Execute(context.Background(), app.AuthenticateInput{AccessToken: old.AccessToken}); !errors.Is(err, app.ErrSessionExpired) {
		t.Errorf("old access token: %v", err)
	}
	if _, err := e.refresh().Execute(context.Background(), app.RefreshInput{RefreshToken: old.RefreshToken}); err == nil {
		t.Error("old refresh token must die")
	}
	if _, err := e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-nova-1"}); err != nil {
		t.Errorf("login with new password: %v", err)
	}
}

// --- admin: autorização ---

func TestOnlyAdminManagesUsers(t *testing.T) {
	ctxs := map[string]context.Context{
		"anonymous": context.Background(),
		"editor":    as("u-editor", domain.RoleEditor),
		"reviewer":  as("u-reviewer", domain.RoleReviewer),
	}
	for who, ctx := range ctxs {
		t.Run(who, func(t *testing.T) {
			e := newEnv()
			e.user(t, "u-editor", "ed@nelisa.org", "senha-certa", domain.RoleEditor)
			want := app.ErrForbidden
			if who == "anonymous" {
				want = app.ErrUnauthenticated
			}
			_, err := e.createUser().Execute(ctx, app.CreateUserInput{Name: "X", Email: "x@nelisa.org", Password: "senha-certa", Role: "admin"})
			check(t, "create", err, want)
			check(t, "reset", e.resetPassword().Execute(ctx, app.ResetPasswordInput{UserID: "u-editor", NewPassword: "senha-nova-1"}), want)
			_, err = e.updateRole().Execute(ctx, app.UpdateRoleInput{UserID: "u-editor", Role: "admin"})
			check(t, "update role (own role, for the editor)", err, want)
			check(t, "delete", e.deleteUser().Execute(ctx, app.DeleteUserInput{UserID: "u-editor"}), want)
			_, err = app.NewListUsers(e.w.Users()).Execute(ctx, app.ListUsersInput{})
			check(t, "list", err, want)

			if u, _ := e.w.Users().FindByID(context.Background(), "u-editor"); u.Role != domain.RoleEditor || u.PasswordHash != "hashed:senha-certa" {
				t.Errorf("refused calls changed the user: %+v", u)
			}
			if len(e.w.Events()) != 0 {
				t.Errorf("refused calls were audited: %+v", e.w.Events())
			}
		})
	}
}

func check(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s: err = %v, want %v", what, err, want)
	}
}

// --- admin: create ---

func TestCreateUser(t *testing.T) {
	e := newEnv()
	ctx := as("admin", domain.RoleAdmin)
	u, err := e.createUser().Execute(ctx, app.CreateUserInput{Name: "Bia", Email: "bia@nelisa.org", Password: "temporaria", Role: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != domain.RoleReviewer || !u.MustChangePassword || u.PasswordHash != "hashed:temporaria" {
		t.Fatalf("user = %+v", u)
	}
	ev := e.w.Events()
	if len(ev) != 1 || ev[0].Action != app.ActionUserCreate || ev[0].ActorID != "admin" || ev[0].TargetID != u.ID || !ev[0].InTx {
		t.Fatalf("events = %+v", ev)
	}
	for _, v := range ev[0].Metadata {
		if v == "temporaria" || v == u.PasswordHash {
			t.Fatal("audit metadata must never carry the password")
		}
	}

	_, err = e.createUser().Execute(ctx, app.CreateUserInput{Name: "Outra", Email: "BIA@nelisa.org", Password: "temporaria", Role: "editor"})
	check(t, "duplicate email (case-insensitive)", err, domain.ErrEmailTaken)
	_, err = e.createUser().Execute(ctx, app.CreateUserInput{Name: "C", Email: "c@nelisa.org", Password: "temporaria", Role: "root"})
	check(t, "bad role", err, domain.ErrInvalidRole)
	_, err = e.createUser().Execute(ctx, app.CreateUserInput{Name: "C", Email: "c@nelisa.org", Password: "curta", Role: "editor"})
	check(t, "short password", err, domain.ErrPasswordTooShort)
	if len(e.w.Events()) != 1 {
		t.Error("failed creations must not be audited")
	}
}

func TestCreateUserRollsBackWhenAuditFails(t *testing.T) {
	e := newEnv()
	e.w.AuditErr = fakes.ErrBoom
	_, err := e.createUser().Execute(as("admin", domain.RoleAdmin), app.CreateUserInput{Name: "Bia", Email: "bia@nelisa.org", Password: "temporaria", Role: "editor"})
	check(t, "create", err, fakes.ErrBoom)
	if _, err := e.w.Users().FindByEmail(context.Background(), "bia@nelisa.org"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatal("user created without its audit event")
	}
}

// --- admin: reset ---

func TestResetPassword(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-antiga", domain.RoleEditor)
	_, _ = e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-antiga"})
	ctx := as("admin", domain.RoleAdmin)

	check(t, "unknown user", e.resetPassword().Execute(ctx, app.ResetPasswordInput{UserID: "nope", NewPassword: "senha-nova-1"}), domain.ErrUserNotFound)
	check(t, "short", e.resetPassword().Execute(ctx, app.ResetPasswordInput{UserID: "u1", NewPassword: "curta"}), domain.ErrPasswordTooShort)

	if err := e.resetPassword().Execute(ctx, app.ResetPasswordInput{UserID: "u1", NewPassword: "senha-nova-1"}); err != nil {
		t.Fatal(err)
	}
	u, _ := e.w.Users().FindByID(context.Background(), "u1")
	if !u.MustChangePassword || u.SessionsValidAfter == nil || u.PasswordHash != "hashed:senha-nova-1" {
		t.Errorf("user = %+v", u)
	}
	if n := len(e.w.TokensOf("u1")); n != 0 {
		t.Errorf("reset must end sessions, %d left", n)
	}
	ev := e.w.Events()
	if len(ev) != 1 || ev[0].Action != app.ActionUserPasswordReset || !ev[0].InTx || len(ev[0].Metadata) != 0 {
		t.Fatalf("events = %+v", ev)
	}
}

func TestResetPasswordRollsBackWhenAuditFails(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-antiga", domain.RoleEditor)
	_, _ = e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-antiga"})
	e.w.AuditErr = fakes.ErrBoom
	check(t, "reset", e.resetPassword().Execute(as("admin", domain.RoleAdmin), app.ResetPasswordInput{UserID: "u1", NewPassword: "senha-nova-1"}), fakes.ErrBoom)
	u, _ := e.w.Users().FindByID(context.Background(), "u1")
	if u.PasswordHash != "hashed:senha-antiga" || len(e.w.TokensOf("u1")) != 1 {
		t.Fatal("reset applied without its audit event")
	}
}

// --- admin: role ---

func TestUpdateRole(t *testing.T) {
	e := newEnv()
	e.user(t, "admin", "admin@nelisa.org", "senha-certa", domain.RoleAdmin)
	e.user(t, "u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	ctx := as("admin", domain.RoleAdmin)

	u, err := e.updateRole().Execute(ctx, app.UpdateRoleInput{UserID: "u1", Role: "reviewer"})
	if err != nil || u.Role != domain.RoleReviewer {
		t.Fatalf("u = %+v, %v", u, err)
	}
	ev := e.w.Events()
	if len(ev) != 1 || ev[0].Action != app.ActionUserRoleChange || ev[0].Metadata["from"] != "editor" || ev[0].Metadata["to"] != "reviewer" || !ev[0].InTx {
		t.Fatalf("events = %+v", ev)
	}
	if e.w.LockCalls == 0 {
		t.Error("role change must lock users")
	}

	// same role: no change, no event
	if _, err := e.updateRole().Execute(ctx, app.UpdateRoleInput{UserID: "u1", Role: "reviewer"}); err != nil || len(e.w.Events()) != 1 {
		t.Errorf("no-op: err=%v events=%d", err, len(e.w.Events()))
	}

	_, err = e.updateRole().Execute(ctx, app.UpdateRoleInput{UserID: "u1", Role: "superuser"})
	check(t, "bad role", err, domain.ErrInvalidRole)
	_, err = e.updateRole().Execute(ctx, app.UpdateRoleInput{UserID: "nope", Role: "editor"})
	check(t, "unknown user", err, domain.ErrUserNotFound)
}

func TestUpdateRoleNeverLeavesNoAdmin(t *testing.T) {
	e := newEnv()
	e.user(t, "admin", "admin@nelisa.org", "senha-certa", domain.RoleAdmin)
	ctx := as("admin", domain.RoleAdmin)
	_, err := e.updateRole().Execute(ctx, app.UpdateRoleInput{UserID: "admin", Role: "editor"})
	check(t, "demote last admin", err, domain.ErrLastAdmin)
	if u, _ := e.w.Users().FindByID(context.Background(), "admin"); u.Role != domain.RoleAdmin {
		t.Fatal("last admin was demoted")
	}

	// with a second admin, demoting oneself is allowed
	e.user(t, "admin2", "admin2@nelisa.org", "senha-certa", domain.RoleAdmin)
	if _, err := e.updateRole().Execute(ctx, app.UpdateRoleInput{UserID: "admin", Role: "editor"}); err != nil {
		t.Fatal(err)
	}
}

// --- admin: delete ---

func TestDeleteUser(t *testing.T) {
	e := newEnv()
	e.user(t, "admin", "admin@nelisa.org", "senha-certa", domain.RoleAdmin)
	e.user(t, "u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	_, _ = e.login().Execute(context.Background(), app.LoginInput{Email: "ana@nelisa.org", Password: "senha-certa"})
	ctx := as("admin", domain.RoleAdmin)

	check(t, "self", e.deleteUser().Execute(ctx, app.DeleteUserInput{UserID: "admin"}), domain.ErrCannotDeleteSelf)
	check(t, "unknown", e.deleteUser().Execute(ctx, app.DeleteUserInput{UserID: "nope"}), domain.ErrUserNotFound)

	if err := e.deleteUser().Execute(ctx, app.DeleteUserInput{UserID: "u1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.w.Users().FindByID(context.Background(), "u1"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Error("user not deleted")
	}
	if len(e.w.TokensOf("u1")) != 0 {
		t.Error("sessions must go with the user")
	}
	ev := e.w.Events()
	if len(ev) != 1 || ev[0].Action != app.ActionUserDelete || ev[0].TargetID != "u1" || !ev[0].InTx {
		t.Fatalf("events = %+v", ev)
	}
}

func TestDeleteUserRollsBackWhenAuditFails(t *testing.T) {
	e := newEnv()
	e.user(t, "u1", "ana@nelisa.org", "senha-certa", domain.RoleEditor)
	e.w.AuditErr = fakes.ErrBoom
	check(t, "delete", e.deleteUser().Execute(as("admin", domain.RoleAdmin), app.DeleteUserInput{UserID: "u1"}), fakes.ErrBoom)
	if _, err := e.w.Users().FindByID(context.Background(), "u1"); err != nil {
		t.Fatal("user deleted without its audit event")
	}
}

func TestListUsers(t *testing.T) {
	e := newEnv()
	e.user(t, "a", "a@nelisa.org", "senha-certa", domain.RoleAdmin)
	e.clk.Advance(time.Second)
	e.user(t, "b", "b@nelisa.org", "senha-certa", domain.RoleEditor)
	got, err := app.NewListUsers(e.w.Users()).Execute(as("a", domain.RoleAdmin), app.ListUsersInput{})
	if err != nil || len(got) != 2 || got[0].ID != "a" {
		t.Fatalf("got %+v, %v", got, err)
	}
}
