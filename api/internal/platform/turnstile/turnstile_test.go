package turnstile_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/platform/turnstile"
)

const secret = "segredo-de-producao"

// siteverify falso: responde body e guarda o formulário recebido.
func fake(t *testing.T, status int, body string) (*httptest.Server, *http.Request) {
	t.Helper()
	got := &http.Request{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		*got = *r
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestVerify(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   error
	}{
		"ok":                {200, `{"success":true,"hostname":"Nelisa.example.org","action":"contribute"}`, nil},
		"other host":        {200, `{"success":true,"hostname":"evil.example","action":"contribute"}`, turnstile.ErrRejected},
		"other action":      {200, `{"success":true,"hostname":"nelisa.example.org","action":"login"}`, turnstile.ErrRejected},
		"no action":         {200, `{"success":true,"hostname":"nelisa.example.org"}`, turnstile.ErrRejected},
		"invalid token":     {200, `{"success":false,"error-codes":["invalid-input-response"]}`, turnstile.ErrRejected},
		"spent token":       {200, `{"success":false,"error-codes":["timeout-or-duplicate"]}`, turnstile.ErrRejected},
		"bad secret":        {200, `{"success":false,"error-codes":["invalid-input-secret"]}`, turnstile.ErrUnavailable},
		"internal error":    {200, `{"success":false,"error-codes":["internal-error"]}`, turnstile.ErrUnavailable},
		"server error":      {502, `bad gateway`, turnstile.ErrUnavailable},
		"garbage":           {200, `<html>`, turnstile.ErrUnavailable},
		"success, no codes": {200, `{"success":false}`, turnstile.ErrUnavailable},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			srv, got := fake(t, c.status, c.body)
			v := turnstile.New(secret, "nelisa.example.org", "contribute", turnstile.WithEndpoint(srv.URL))
			err := v.Verify(context.Background(), "token-do-leitor")
			if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("Verify = %v; want %v", err, c.want)
			}
			if got.PostForm.Get("secret") != secret || got.PostForm.Get("response") != "token-do-leitor" {
				t.Errorf("formulário = %v", got.PostForm)
			}
			if got.PostForm.Has("remoteip") {
				t.Errorf("o IP do Leitor não vai para a Cloudflare")
			}
		})
	}
}

func TestVerifyRejectsBadTokensWithoutCalling(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer srv.Close()
	v := turnstile.New(secret, "x", "contribute", turnstile.WithEndpoint(srv.URL))
	for _, tok := range []string{"", strings.Repeat("a", turnstile.MaxTokenLength+1)} {
		if err := v.Verify(context.Background(), tok); !errors.Is(err, turnstile.ErrRejected) {
			t.Errorf("token de %d bytes = %v", len(tok), err)
		}
	}
	if calls != 0 {
		t.Errorf("chamou o siteverify %d vezes", calls)
	}
}

func TestVerifyUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	v := turnstile.New(secret, "x", "contribute", turnstile.WithEndpoint(srv.URL))
	if err := v.Verify(context.Background(), "t"); !errors.Is(err, turnstile.ErrUnavailable) {
		t.Errorf("siteverify fora do ar = %v; want ErrUnavailable (nunca aprovado)", err)
	}
}

func TestTestSecretSkipsHostAndAction(t *testing.T) {
	// Com a chave de teste a Cloudflare devolve example.com e nenhuma action.
	srv, _ := fake(t, 200, `{"success":true,"hostname":"example.com"}`)
	v := turnstile.New("1x0000000000000000000000000000000AA", "localhost", "contribute", turnstile.WithEndpoint(srv.URL))
	if err := v.Verify(context.Background(), "XXXX.DUMMY.TOKEN.XXXX"); err != nil {
		t.Errorf("chave de teste = %v", err)
	}
	if !turnstile.IsTestSecret("2x0000000000000000000000000000000AA") || turnstile.IsTestSecret(secret) {
		t.Error("IsTestSecret")
	}
}
