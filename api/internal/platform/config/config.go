// Package config lê e valida toda a configuração do processo a partir de
// variáveis de ambiente, uma vez, no boot. Nenhum outro pacote lê os.Getenv.
package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// MinSecretBytes é o tamanho mínimo do segredo de assinatura dos tokens.
const MinSecretBytes = 32

// Config é a configuração validada do processo.
type Config struct {
	// DatabaseURL é a URL do Postgres (postgres://user:pass@host:5432/db).
	DatabaseURL string
	// PublicBaseURL é a URL pública do site, sem barra no fim. Todo link
	// absoluto sai daqui: nenhum domínio fixo no código.
	PublicBaseURL *url.URL
	// ListenAddr é onde a API escuta (padrão :8080).
	ListenAddr string
	// JWTSecret assina os tokens de acesso. Obrigatório só no modo api.
	JWTSecret string
	// TrustedProxies são as redes cujo CF-Connecting-IP é aceito como IP do
	// cliente. Padrão: redes privadas e loopback (Traefik/cloudflared).
	TrustedProxies []netip.Prefix
	// SecureCookies desliga só com COOKIE_INSECURE=true (dev em http).
	SecureCookies bool
}

// Getenv é a assinatura de os.Getenv, injetada para os testes.
type Getenv func(string) string

// placeholderSecrets são valores de exemplo que nunca podem ir para
// produção.
var placeholderSecrets = []string{"change-me", "changeme", "dev-secret", "secret", "example"}

// Load lê a configuração. requireSecret indica se o modo em execução assina
// tokens (api) e portanto precisa de JWT_SECRET.
func Load(getenv Getenv, requireSecret bool) (Config, error) {
	var errs []error
	c := Config{
		DatabaseURL:   getenv("DATABASE_URL"),
		ListenAddr:    getenv("LISTEN_ADDR"),
		JWTSecret:     getenv("JWT_SECRET"),
		SecureCookies: getenv("COOKIE_INSECURE") != "true",
	}
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL é obrigatório"))
	}

	base, err := parseBaseURL(getenv("PUBLIC_BASE_URL"))
	if err != nil {
		errs = append(errs, err)
	}
	c.PublicBaseURL = base

	if requireSecret {
		switch {
		case c.JWTSecret == "":
			errs = append(errs, errors.New("JWT_SECRET é obrigatório"))
		case base != nil && base.Scheme == "https" && isPlaceholder(c.JWTSecret):
			errs = append(errs, errors.New("JWT_SECRET é um valor de exemplo; gere um segredo real"))
		case base != nil && base.Scheme == "https" && len(c.JWTSecret) < MinSecretBytes:
			errs = append(errs, fmt.Errorf("JWT_SECRET precisa de pelo menos %d bytes", MinSecretBytes))
		}
	}

	proxies, err := parseCIDRs(getenv("TRUSTED_PROXY_CIDRS"))
	if err != nil {
		errs = append(errs, err)
	}
	c.TrustedProxies = proxies

	if base != nil && base.Scheme == "https" && !c.SecureCookies {
		errs = append(errs, errors.New("COOKIE_INSECURE=true não é permitido com PUBLIC_BASE_URL https"))
	}
	return c, errors.Join(errs...)
}

func parseBaseURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("PUBLIC_BASE_URL é obrigatório")
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("PUBLIC_BASE_URL inválido: %q", raw)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("PUBLIC_BASE_URL deve ser só esquema e host: %q", raw)
	}
	return u, nil
}

var defaultProxies = []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "::1/128", "fc00::/7"}

func parseCIDRs(raw string) ([]netip.Prefix, error) {
	items := defaultProxies
	if strings.TrimSpace(raw) != "" {
		items = strings.Split(raw, ",")
	}
	out := make([]netip.Prefix, 0, len(items))
	for _, s := range items {
		p, err := netip.ParsePrefix(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS inválido: %q", s)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

func isPlaceholder(s string) bool {
	l := strings.ToLower(s)
	for _, p := range placeholderSecrets {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

// URL monta um link absoluto a partir de PUBLIC_BASE_URL.
func (c Config) URL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.PublicBaseURL.String() + path
}
