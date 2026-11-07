package store_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
	"github.com/mphennrichs/nelisa/api/internal/team/app"
	"github.com/mphennrichs/nelisa/api/internal/team/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
	"github.com/mphennrichs/nelisa/api/internal/team/store"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

const (
	id1 = "00000000-0000-4000-8000-000000000001"
	id2 = "00000000-0000-4000-8000-000000000002"
	id3 = "00000000-0000-4000-8000-000000000003"
)

func mkUser(t *testing.T, users *store.Users, id, email string, role domain.Role) domain.User {
	t.Helper()
	u, err := domain.NewUser(id, "Pessoa", email, "hash-"+id, role, true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := users.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestUsersCreateAndFind(t *testing.T) {
	pool, _ := testdb.New(t)
	users := store.NewUsers(pool)
	ctx := context.Background()
	u := mkUser(t, users, id1, "Ana@Nelisa.org", domain.RoleReviewer)

	got, err := users.FindByEmail(ctx, "ana@NELISA.org")
	if err != nil {
		t.Fatalf("email lookup is case-insensitive: %v", err)
	}
	if got.ID != u.ID || got.Name != u.Name || got.Role != domain.RoleReviewer || !got.MustChangePassword || !got.CreatedAt.Equal(t0) || got.SessionsValidAfter != nil {
		t.Errorf("got %+v", got)
	}
	if _, err := users.FindByID(ctx, id1); err != nil {
		t.Error(err)
	}

	dup, _ := domain.NewUser(id2, "Outra", "ANA@nelisa.org", "h", domain.RoleEditor, false, t0)
	if err := users.Create(ctx, dup); !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("duplicate email: %v", err)
	}
	for _, id := range []string{id3, "nao-e-uuid"} {
		if _, err := users.FindByID(ctx, id); !errors.Is(err, domain.ErrUserNotFound) {
			t.Errorf("FindByID(%q): %v", id, err)
		}
	}
	if _, err := users.FindByEmail(ctx, "ninguem@nelisa.org"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("unknown email: %v", err)
	}
}

func TestUsersCountListRoleDelete(t *testing.T) {
	pool, _ := testdb.New(t)
	users := store.NewUsers(pool)
	tokens := store.NewRefreshTokens(pool)
	ctx := context.Background()
	mkUser(t, users, id1, "a@nelisa.org", domain.RoleAdmin)
	mkUser(t, users, id2, "b@nelisa.org", domain.RoleEditor)

	if n, _ := users.Count(ctx); n != 2 {
		t.Errorf("count = %d", n)
	}
	if n, _ := users.CountAdmins(ctx); n != 1 {
		t.Errorf("admins = %d", n)
	}
	list, err := users.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %v, %v", list, err)
	}

	u, err := users.UpdateRole(ctx, id2, domain.RoleAdmin)
	if err != nil || u.Role != domain.RoleAdmin {
		t.Fatalf("update role: %+v, %v", u, err)
	}
	if _, err := users.UpdateRole(ctx, id3, domain.RoleAdmin); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("update unknown: %v", err)
	}

	if err := tokens.Create(ctx, domain.NewRefreshToken(id3, id2, "hash-b", t0)); err != nil {
		t.Fatal(err)
	}
	if err := users.Delete(ctx, id2); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.FindByHash(ctx, "hash-b"); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Errorf("sessions must be deleted with the user: %v", err)
	}
	if err := users.Delete(ctx, id2); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("delete twice: %v", err)
	}
}

func TestUsersUpdatePasswordGuardsConcurrentChange(t *testing.T) {
	pool, _ := testdb.New(t)
	users := store.NewUsers(pool)
	ctx := context.Background()
	u := mkUser(t, users, id1, "a@nelisa.org", domain.RoleEditor)
	prev := u.PasswordHash

	u.ChangePassword("novo-hash", t0.Add(1500*time.Microsecond))
	if err := users.UpdatePassword(ctx, u, prev); err != nil {
		t.Fatal(err)
	}
	got, _ := users.FindByID(ctx, id1)
	if got.PasswordHash != "novo-hash" || got.MustChangePassword || got.SessionsValidAfter == nil || !got.SessionsValidAfter.Equal(t0.Add(time.Millisecond)) {
		t.Errorf("got %+v", got)
	}
	// a writer that read the old hash loses
	u.ResetPassword("outro-hash", t0)
	if err := users.UpdatePassword(ctx, u, prev); !errors.Is(err, domain.ErrCredentialsChanged) {
		t.Errorf("stale write: %v", err)
	}
	u.ID = id2
	if err := users.UpdatePassword(ctx, u, prev); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("unknown user: %v", err)
	}
}

func TestRefreshTokens(t *testing.T) {
	pool, _ := testdb.New(t)
	users := store.NewUsers(pool)
	tokens := store.NewRefreshTokens(pool)
	ctx := context.Background()
	mkUser(t, users, id1, "a@nelisa.org", domain.RoleEditor)

	tok := domain.NewRefreshToken(id2, id1, "hash-1", t0)
	if err := tokens.Create(ctx, tok); err != nil {
		t.Fatal(err)
	}
	got, err := tokens.FindByHash(ctx, "hash-1")
	if err != nil || got.ID != id2 || got.UserID != id1 || !got.ExpiresAt.Equal(tok.ExpiresAt) || got.RevokedAt != nil {
		t.Fatalf("got %+v, %v", got, err)
	}

	if err := tokens.Revoke(ctx, id2, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := tokens.Revoke(ctx, id2, t0.Add(2*time.Minute)); !errors.Is(err, domain.ErrRefreshTokenReused) {
		t.Errorf("second revoke: %v", err)
	}
	got, _ = tokens.FindByHash(ctx, "hash-1")
	if got.RevokedAt == nil || !got.RevokedAt.Equal(t0.Add(time.Minute)) {
		t.Errorf("revoked_at = %v", got.RevokedAt)
	}
	if err := tokens.Delete(ctx, id2); err != nil {
		t.Fatal(err)
	}
	if err := tokens.Revoke(ctx, id2, t0); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
		t.Errorf("revoke deleted: %v", err)
	}

	_ = tokens.Create(ctx, domain.NewRefreshToken(id2, id1, "hash-2", t0))
	_ = tokens.Create(ctx, domain.NewRefreshToken(id3, id1, "hash-3", t0))
	if err := tokens.DeleteAllForUser(ctx, id1); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"hash-2", "hash-3"} {
		if _, err := tokens.FindByHash(ctx, h); !errors.Is(err, domain.ErrRefreshTokenInvalid) {
			t.Errorf("%s survived: %v", h, err)
		}
	}
}

// nopAudit aceita tudo: aqui o que se testa é a trava no banco real.
type nopAudit struct{}

func (nopAudit) Record(context.Context, app.AuditEvent) error { return nil }

func TestConcurrentBootstrapCreatesOneAdmin(t *testing.T) {
	pool, _ := testdb.New(t)
	users := store.NewUsers(pool)
	tokens := store.NewRefreshTokens(pool)
	clk := &clock.Fixed{T: t0}
	sessions := app.NewSessions(auth.NewTokens(strings.Repeat("s", 32), clk), tokens, clk)
	uc := app.NewBootstrap(users, fakes.Hasher{}, sessions, db.NewTxRunner(pool), nopAudit{}, clk, "segredo-de-bootstrap")

	const n = 5
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = uc.Execute(context.Background(), app.BootstrapInput{
				Token: "segredo-de-bootstrap", Name: "Ana", Email: "ana" + string(rune('a'+i)) + "@nelisa.org", Password: "senha-forte",
			})
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, app.ErrAlreadyBootstrapped):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if count, _ := users.Count(context.Background()); ok != 1 || count != 1 {
		t.Fatalf("successes = %d, users = %d; want exactly one admin", ok, count)
	}
}
