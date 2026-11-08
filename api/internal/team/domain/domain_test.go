package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func TestParseRole(t *testing.T) {
	for _, r := range []string{"editor", "reviewer", "admin"} {
		if _, err := ParseRole(r); err != nil {
			t.Errorf("ParseRole(%q) = %v", r, err)
		}
	}
	for _, r := range []string{"", "Admin", "root", "reader"} {
		if _, err := ParseRole(r); !errors.Is(err, ErrInvalidRole) {
			t.Errorf("ParseRole(%q) = %v, want ErrInvalidRole", r, err)
		}
	}
}

func TestOnlyAdminManagesUsers(t *testing.T) {
	if CanManageUsers(RoleEditor) || CanManageUsers(RoleReviewer) || !CanManageUsers(RoleAdmin) {
		t.Fatal("only admin may manage users")
	}
}

func TestNewUserValidates(t *testing.T) {
	cases := map[string]struct {
		name, email string
		role        Role
		want        error
	}{
		"empty name":   {" ", "a@b.org", RoleEditor, ErrInvalidName},
		"long name":    {strings.Repeat("a", 121), "a@b.org", RoleEditor, ErrInvalidName},
		"bad email":    {"Ana", "ana", RoleEditor, ErrInvalidEmail},
		"spaced email": {"Ana", "a b@c.org", RoleEditor, ErrInvalidEmail},
		"bad role":     {"Ana", "a@b.org", Role("root"), ErrInvalidRole},
	}
	for name, c := range cases {
		if _, err := NewUser("id", c.name, c.email, "h", c.role, false, t0); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	u, err := NewUser("id", "  Ana  ", " Ana@B.org ", "h", RoleEditor, true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "Ana" || u.Email != "Ana@B.org" || !u.MustChangePassword || u.SessionsValidAfter != nil {
		t.Errorf("user = %+v", u)
	}
}

func TestValidatePassword(t *testing.T) {
	for _, pw := range []string{"", "1234567", "çãõéíóú"} {
		if !errors.Is(ValidatePassword(pw), ErrPasswordTooShort) {
			t.Errorf("%q should be too short", pw)
		}
	}
	for _, pw := range []string{"12345678", "çãõéíóúà", strings.Repeat("a", 72)} {
		if err := ValidatePassword(pw); err != nil {
			t.Errorf("%q: %v", pw, err)
		}
	}
	if !errors.Is(ValidatePassword(strings.Repeat("a", 73)), ErrPasswordTooLong) {
		t.Error("73 bytes should be too long")
	}
}

func TestPasswordChangesEndSessions(t *testing.T) {
	u, _ := NewUser("id", "Ana", "a@b.org", "old", RoleEditor, true, t0)
	at := t0.Add(time.Hour + 1500*time.Microsecond)
	u.ChangePassword("new", at)
	if u.MustChangePassword || u.PasswordHash != "new" {
		t.Errorf("change: %+v", u)
	}
	if u.SessionValid(at.Add(-time.Second)) {
		t.Error("token issued before the change must be refused")
	}
	if !u.SessionValid(at.Truncate(time.Millisecond)) {
		t.Error("token issued at the change (ms precision) must be accepted")
	}
	u.ResetPassword("temp", at.Add(time.Hour))
	if !u.MustChangePassword || u.SessionValid(at) {
		t.Errorf("reset must mark temporary and end sessions: %+v", u)
	}
}

func TestDeactivationBlocksSessionsUntilReactivated(t *testing.T) {
	u, _ := NewUser("id", "Ana", "a@b.org", "h", RoleEditor, false, t0)
	if !u.Active() {
		t.Fatal("new user must be active")
	}
	at := t0.Add(time.Hour)
	u.Deactivate(at)
	if u.Active() || u.DeactivatedAt == nil {
		t.Fatalf("deactivate: %+v", u)
	}
	if u.SessionValid(at.Add(time.Hour)) {
		t.Error("a deactivated account has no valid session, not even a new one")
	}
	first := *u.DeactivatedAt
	u.Deactivate(at.Add(time.Hour))
	if !u.DeactivatedAt.Equal(first) {
		t.Error("deactivating twice must keep the first date")
	}
	u.Reactivate()
	if !u.Active() {
		t.Fatal("reactivate")
	}
	if u.SessionValid(at.Add(-time.Second)) {
		t.Error("sessions from before the deactivation stay dead after reactivating")
	}
	if !u.SessionValid(at.Add(2 * time.Hour)) {
		t.Error("a new session after reactivating must be valid")
	}
}

func TestRefreshTokenLifetime(t *testing.T) {
	tok := NewRefreshToken("t", "u", "h", t0)
	if tok.IsExpired(t0.Add(RefreshTokenTTL-time.Second)) || !tok.IsExpired(t0.Add(RefreshTokenTTL)) {
		t.Error("expiry boundary wrong")
	}
	if tok.ReusedAfterGrace(t0.Add(time.Hour)) {
		t.Error("unspent token is not reuse")
	}
	spent := t0
	tok.RevokedAt = &spent
	if tok.ReusedAfterGrace(t0.Add(ReuseGrace - time.Second)) {
		t.Error("within grace is a parallel refresh, not reuse")
	}
	if !tok.ReusedAfterGrace(t0.Add(ReuseGrace)) {
		t.Error("after grace is reuse")
	}
}
