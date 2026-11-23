// Package turnstile valida no servidor o token do Cloudflare Turnstile (o
// captcha do formulário de Contribuição) com o siteverify da Cloudflare.
// Não conhece nenhum módulo: o contribution o recebe por porta.
package turnstile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// SiteverifyURL é o endpoint oficial de validação.
const SiteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// MaxTokenLength é o maior token que a Cloudflare emite.
const MaxTokenLength = 2048

var (
	// ErrRejected: o token não vale (falso, vencido, já usado, de outro
	// site ou de outra ação). Quem mandou deve resolver o desafio de novo.
	ErrRejected = errors.New("turnstile: token recusado")
	// ErrUnavailable: não deu para validar (siteverify fora do ar, resposta
	// estranha ou segredo errado). Nunca vale como aprovado.
	ErrUnavailable = errors.New("turnstile: validação indisponível")
)

// testSecrets são as chaves de teste públicas da Cloudflare. Com elas o
// siteverify devolve sempre hostname example.com e nenhuma action, então
// essas duas checagens não se aplicam. A config recusa essas chaves em
// https.
var testSecrets = []string{
	"1x0000000000000000000000000000000AA",
	"2x0000000000000000000000000000000AA",
	"3x0000000000000000000000000000000AA",
}

// IsTestSecret diz se secret é uma chave de teste da Cloudflare.
func IsTestSecret(secret string) bool { return slices.Contains(testSecrets, secret) }

// Verifier valida tokens de um widget, conferindo o site e a ação.
type Verifier struct {
	secret   string
	hostname string
	action   string
	endpoint string
	client   *http.Client
}

// Option ajusta o Verifier (nos testes, o endpoint).
type Option func(*Verifier)

// WithEndpoint troca o siteverify (testes).
func WithEndpoint(u string) Option { return func(v *Verifier) { v.endpoint = u } }

// New monta o verificador. hostname é o host de PUBLIC_BASE_URL e action,
// a ação declarada pelo widget.
func New(secret, hostname, action string, opts ...Option) *Verifier {
	v := &Verifier{
		secret: secret, hostname: strings.ToLower(hostname), action: action,
		endpoint: SiteverifyURL, client: &http.Client{Timeout: 10 * time.Second},
	}
	for _, o := range opts {
		o(v)
	}
	return v
}

type siteverifyResponse struct {
	Success    bool     `json:"success"`
	Hostname   string   `json:"hostname"`
	Action     string   `json:"action"`
	ErrorCodes []string `json:"error-codes"`
}

// Erros do siteverify que são culpa do token, não da configuração nem da
// Cloudflare.
var tokenErrors = []string{"missing-input-response", "invalid-input-response", "timeout-or-duplicate"}

// Verify devolve nil só se a Cloudflare confirmou o token para este site e
// esta ação; ErrRejected se o token não vale; ErrUnavailable se não deu
// para saber.
func (v *Verifier) Verify(ctx context.Context, token string) error {
	if token == "" || len(token) > MaxTokenLength {
		return ErrRejected
	}
	form := url.Values{"secret": {v.secret}, "response": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: siteverify respondeu %d", ErrUnavailable, resp.StatusCode)
	}
	var out siteverifyResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return fmt.Errorf("%w: resposta ilegível: %v", ErrUnavailable, err)
	}
	if !out.Success {
		for _, c := range out.ErrorCodes {
			if slices.Contains(tokenErrors, c) {
				return fmt.Errorf("%w: %s", ErrRejected, c)
			}
		}
		// Segredo errado, requisição malformada ou erro interno: não é
		// culpa de quem mandou, e também não aprova.
		return fmt.Errorf("%w: %s", ErrUnavailable, strings.Join(out.ErrorCodes, ","))
	}
	if IsTestSecret(v.secret) {
		return nil
	}
	if !strings.EqualFold(out.Hostname, v.hostname) {
		return fmt.Errorf("%w: hostname %q", ErrRejected, out.Hostname)
	}
	if out.Action != v.action {
		return fmt.Errorf("%w: action %q", ErrRejected, out.Action)
	}
	return nil
}
