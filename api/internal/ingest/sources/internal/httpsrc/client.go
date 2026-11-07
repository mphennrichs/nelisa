// Package httpsrc é a infraestrutura HTTP comum aos adaptadores das fontes
// oficiais (Câmara e Senado): timeout, User-Agent, limite de concorrência por
// domínio, limite de tamanho da resposta e tradução de falhas para
// app.ErrNotFound / app.ErrUnavailable. Não tem regra de negócio.
package httpsrc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
)

// Padrões de qualquer adaptador.
const (
	UserAgent          = "Nelisa/1.0 (+https://github.com/mphennrichs/nelisa)"
	DefaultTimeout     = 15 * time.Second
	DefaultConcurrency = 2
	DefaultMaxBody     = 10 << 20 // 10 MiB
	// maxRetryAfter limita a espera numa única nova tentativa após 429/503.
	// Esperas maiores ficam para quem chama (River).
	maxRetryAfter = 5 * time.Second
)

// Config ajusta o cliente; campos zerados usam os padrões acima.
type Config struct {
	Timeout     time.Duration
	Concurrency int
	MaxBody     int64
}

// Client faz GETs de JSON numa API oficial. Um Client por domínio: o
// semáforo limita as requisições simultâneas àquele domínio.
type Client struct {
	http    *http.Client
	base    *url.URL
	sem     chan struct{}
	timeout time.Duration
	maxBody int64
}

// New cria o cliente. hc nil usa um http.Client próprio; baseURL é a raiz da
// API (ex.: "https://dadosabertos.camara.leg.br/api/v2").
func New(hc *http.Client, baseURL string, cfg Config) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("URL base inválida: %q", baseURL)
	}
	if hc == nil {
		hc = &http.Client{}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = DefaultConcurrency
	}
	if cfg.MaxBody <= 0 {
		cfg.MaxBody = DefaultMaxBody
	}
	return &Client{
		http:    hc,
		base:    u,
		sem:     make(chan struct{}, cfg.Concurrency),
		timeout: cfg.Timeout,
		maxBody: cfg.MaxBody,
	}, nil
}

// URL monta o endereço absoluto de path (relativo à base) com a query.
func (c *Client) URL(path string, q url.Values) string {
	u := *c.base
	u.Path = c.base.Path + path
	u.RawQuery = q.Encode()
	return u.String()
}

// SameHost diz se raw aponta para o mesmo host da base. Usado antes de
// seguir links que vêm na resposta (paginação): nunca seguimos URL externa.
func (c *Client) SameHost(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Host == c.base.Host && (u.Scheme == "https" || u.Scheme == c.base.Scheme)
}

// GetJSON busca rawURL e decodifica o corpo em out. 404 vira ErrNotFound;
// rede, timeout, 5xx, corpo grande demais ou JSON inválido viram
// ErrUnavailable. Faz no máximo uma nova tentativa, só em 429/503 com
// Retry-After curto.
func (c *Client) GetJSON(ctx context.Context, rawURL string, out any) error {
	for attempt := 0; ; attempt++ {
		wait, err := c.getOnce(ctx, rawURL, out)
		if wait <= 0 || attempt > 0 {
			return err
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return fmt.Errorf("%w: %s: %v", app.ErrUnavailable, rawURL, ctx.Err())
		case <-t.C:
		}
	}
}

// getOnce faz uma requisição. Devolve wait > 0 quando vale tentar de novo
// depois desse tempo (429/503 com Retry-After curto).
func (c *Client) getOnce(ctx context.Context, rawURL string, out any) (time.Duration, error) {
	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return 0, fmt.Errorf("%w: %s: %v", app.ErrUnavailable, rawURL, ctx.Err())
	}
	defer func() { <-c.sem }()

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, fmt.Errorf("%w: requisição inválida %s: %v", app.ErrUnavailable, rawURL, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %v", app.ErrUnavailable, rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return 0, fmt.Errorf("%w: %s", app.ErrNotFound, rawURL)
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
		err := fmt.Errorf("%w: %s: HTTP %d", app.ErrUnavailable, rawURL, resp.StatusCode)
		return retryAfter(resp.Header.Get("Retry-After")), err
	case resp.StatusCode != http.StatusOK:
		return 0, fmt.Errorf("%w: %s: HTTP %d", app.ErrUnavailable, rawURL, resp.StatusCode)
	}

	// Lê um byte além do limite para distinguir "cabe" de "estourou".
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return 0, fmt.Errorf("%w: %s: lendo resposta: %v", app.ErrUnavailable, rawURL, err)
	}
	if int64(len(body)) > c.maxBody {
		return 0, fmt.Errorf("%w: %s: resposta maior que %d bytes", app.ErrUnavailable, rawURL, c.maxBody)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return 0, fmt.Errorf("%w: %s: JSON inválido: %v", app.ErrUnavailable, rawURL, err)
	}
	return 0, nil
}

// retryAfter interpreta Retry-After em segundos; só aceita esperas curtas.
func retryAfter(h string) time.Duration {
	s, err := strconv.Atoi(h)
	if err != nil || s < 0 {
		return 0
	}
	d := time.Duration(s) * time.Second
	if d > maxRetryAfter {
		return 0
	}
	if d == 0 {
		d = 100 * time.Millisecond
	}
	return d
}

// Invalid embrulha uma resposta que não passou na validação: para quem
// chama, a fonte está indisponível (nunca gravamos dado parcial).
func Invalid(format string, args ...any) error {
	return fmt.Errorf("%w: resposta inválida: %s", app.ErrUnavailable, fmt.Sprintf(format, args...))
}

// IsNotFound é atalho para errors.Is(err, app.ErrNotFound).
func IsNotFound(err error) bool { return errors.Is(err, app.ErrNotFound) }
