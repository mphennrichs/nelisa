// Package domain tem as regras das Contribuições (CONTEXT.md,
// "Contribuições"): o que o Leitor pode mandar, o código de acompanhamento
// e a Fonte oficial. Só Go padrão.
//
// O Nelisa nunca abre, baixa nem pré-visualiza um link mandado pelo Leitor
// (AGENTS.md, "nada de URL arbitrária"): aqui só se valida o texto.
package domain

import (
	"errors"
	"net/netip"
	"net/url"
	"strings"
)

// MaxLinkLen limita um link mandado pelo Leitor.
const MaxLinkLen = 2000

var (
	// ErrInvalidLink: link que não é https, com host DNS, sem usuário,
	// senha ou porta.
	ErrInvalidLink = errors.New("contribution: link inválido")
	// ErrInvalidDomain: domínio que não é um nome DNS com pelo menos dois
	// rótulos.
	ErrInvalidDomain = errors.New("contribution: domínio inválido")
)

// Link é um link já validado.
type Link struct {
	// URL é o link como será guardado.
	URL string
	// Host é o domínio, em minúsculas e sem ponto final.
	Host string
}

// ParseLink valida um link mandado pelo Leitor: https, até MaxLinkLen
// caracteres, host DNS válido (IP literal é recusado), sem usuário, senha
// ou porta.
func ParseLink(raw string) (Link, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > MaxLinkLen || strings.ContainsAny(raw, " \t\r\n") {
		return Link{}, ErrInvalidLink
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.User != nil || u.Port() != "" {
		return Link{}, ErrInvalidLink
	}
	host, err := NormalizeDomain(u.Hostname())
	if err != nil {
		return Link{}, ErrInvalidLink
	}
	return Link{URL: u.String(), Host: host}, nil
}

// NormalizeDomain valida e normaliza um domínio: minúsculas, sem ponto
// final, rótulos DNS (letras, dígitos e hífen, 1 a 63 caracteres, sem hífen
// nas pontas), pelo menos dois rótulos, até 253 caracteres. IP é recusado.
// Domínio com acento precisa vir em punycode (xn--).
func NormalizeDomain(raw string) (string, error) {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if d == "" || len(d) > 253 {
		return "", ErrInvalidDomain
	}
	if _, err := netip.ParseAddr(strings.Trim(d, "[]")); err == nil {
		return "", ErrInvalidDomain
	}
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return "", ErrInvalidDomain
	}
	for _, l := range labels {
		if !validLabel(l) {
			return "", ErrInvalidDomain
		}
	}
	// O último rótulo (TLD) nunca é só número.
	if strings.Trim(labels[len(labels)-1], "0123456789") == "" {
		return "", ErrInvalidDomain
	}
	return d, nil
}

func validLabel(l string) bool {
	if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for _, r := range l {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

// InDomain diz se host é domain ou um subdomínio dele. Os dois já
// normalizados.
func InDomain(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}
