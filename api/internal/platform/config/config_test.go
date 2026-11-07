package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) Getenv {
	return func(k string) string { return m[k] }
}

func base() map[string]string {
	return map[string]string{
		"DATABASE_URL":    "postgres://x",
		"PUBLIC_BASE_URL": "https://nelisa.example.org/",
		"JWT_SECRET":      strings.Repeat("k", 40),
	}
}

func TestLoadValid(t *testing.T) {
	c, err := Load(env(base()), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := c.URL("proposicao/1"); got != "https://nelisa.example.org/proposicao/1" {
		t.Errorf("URL = %q", got)
	}
	if c.ListenAddr != ":8080" || !c.SecureCookies || len(c.TrustedProxies) == 0 {
		t.Errorf("defaults not applied: %+v", c)
	}
}

func TestLoadRejects(t *testing.T) {
	cases := map[string]func(m map[string]string){
		"missing database":       func(m map[string]string) { delete(m, "DATABASE_URL") },
		"missing base url":       func(m map[string]string) { delete(m, "PUBLIC_BASE_URL") },
		"base url with path":     func(m map[string]string) { m["PUBLIC_BASE_URL"] = "https://x.org/app" },
		"base url bad scheme":    func(m map[string]string) { m["PUBLIC_BASE_URL"] = "ftp://x.org" },
		"missing secret":         func(m map[string]string) { delete(m, "JWT_SECRET") },
		"placeholder secret":     func(m map[string]string) { m["JWT_SECRET"] = "change-me-" + strings.Repeat("x", 40) },
		"short secret on https":  func(m map[string]string) { m["JWT_SECRET"] = "short" },
		"insecure cookie https":  func(m map[string]string) { m["COOKIE_INSECURE"] = "true" },
		"bad trusted proxy cidr": func(m map[string]string) { m["TRUSTED_PROXY_CIDRS"] = "nope" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := base()
			mutate(m)
			if _, err := Load(env(m), true); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadDevAllowsWeakSecretOverHTTP(t *testing.T) {
	m := base()
	m["PUBLIC_BASE_URL"] = "http://localhost:3000"
	m["JWT_SECRET"] = "dev-secret"
	m["COOKIE_INSECURE"] = "true"
	if _, err := Load(env(m), true); err != nil {
		t.Fatalf("dev config should load: %v", err)
	}
}

func TestLoadWorkerNeedsNoSecret(t *testing.T) {
	m := base()
	delete(m, "JWT_SECRET")
	if _, err := Load(env(m), false); err != nil {
		t.Fatalf("worker config should load without secret: %v", err)
	}
}
