package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func TestRoleAtLeast(t *testing.T) {
	cases := []struct {
		r, min Role
		want   bool
	}{
		{RoleAdmin, RoleEditor, true},
		{RoleAdmin, RoleAdmin, true},
		{RoleReviewer, RoleEditor, true},
		{RoleReviewer, RoleAdmin, false},
		{RoleEditor, RoleReviewer, false},
		{Role("root"), RoleEditor, false},
		{Role(""), RoleEditor, false},
	}
	for _, c := range cases {
		if got := c.r.AtLeast(c.min); got != c.want {
			t.Errorf("%q.AtLeast(%q) = %v", c.r, c.min, got)
		}
	}
}

func TestTokensRoundTrip(t *testing.T) {
	clk := &clock.Fixed{T: t0}
	tok := NewTokens(strings.Repeat("s", 32), clk)
	s, exp, err := tok.Issue("u1", RoleReviewer)
	if err != nil {
		t.Fatal(err)
	}
	if !exp.Equal(t0.Add(AccessTokenTTL)) {
		t.Errorf("exp = %v", exp)
	}
	c, err := tok.Verify(s)
	if err != nil {
		t.Fatal(err)
	}
	if c.UserID != "u1" || c.Role != RoleReviewer || !c.IssuedAt.Equal(t0) {
		t.Errorf("claims = %+v", c)
	}
}

func TestTokensKeepMillisecondIssuedAt(t *testing.T) {
	at := t0.Add(123 * time.Millisecond)
	tok := NewTokens(strings.Repeat("s", 32), &clock.Fixed{T: at})
	s, _, _ := tok.Issue("u1", RoleEditor)
	c, err := tok.Verify(s)
	if err != nil || !c.IssuedAt.Equal(at) {
		t.Fatalf("iat = %v, err = %v; want %v", c.IssuedAt, err, at)
	}
}

func TestTokensRejectInvalid(t *testing.T) {
	clk := &clock.Fixed{T: t0}
	tok := NewTokens(strings.Repeat("s", 32), clk)
	good, _, _ := tok.Issue("u1", RoleAdmin)
	other, _, _ := NewTokens(strings.Repeat("x", 32), clk).Issue("u1", RoleAdmin)
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "u1", "iss": issuer, "aud": audience, "iat": t0.Unix(), "exp": t0.Add(time.Hour).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)

	for name, s := range map[string]string{"garbage": "abc", "wrong secret": other, "alg none": none, "empty": ""} {
		if _, err := tok.Verify(s); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	clk.Advance(AccessTokenTTL + time.Second)
	if _, err := tok.Verify(good); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expired: err = %v", err)
	}
}

func TestRequireRole(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := RequireRole(RoleAdmin)(ok)
	cases := []struct {
		name string
		ctx  context.Context
		code int
	}{
		{"no actor", context.Background(), http.StatusUnauthorized},
		{"editor", WithActor(context.Background(), Actor{UserID: "u", Role: RoleEditor}), http.StatusForbidden},
		{"admin", WithActor(context.Background(), Actor{UserID: "u", Role: RoleAdmin}), http.StatusNoContent},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil).WithContext(c.ctx))
		if w.Code != c.code {
			t.Errorf("%s: status %d, want %d", c.name, w.Code, c.code)
		}
	}
}
