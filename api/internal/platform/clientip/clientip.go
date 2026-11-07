// Package clientip extrai o IP real do cliente atrás do Cloudflare Tunnel +
// Traefik.
//
// RemoteAddr é sempre o proxy, e X-Forwarded-For/X-Real-IP chegam reescritos
// com o IP do cloudflared: ficam iguais para todo mundo. Só o
// CF-Connecting-IP, escrito pela borda da Cloudflare, traz o cliente. Ele só
// é aceito quando a conexão vem de um proxy confiável; vindo de qualquer
// outro lugar, o cliente escolheria o próprio "IP" e escaparia dos limites.
package clientip

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Resolver resolve o IP do cliente dado o conjunto de proxies confiáveis.
type Resolver struct {
	trusted []netip.Prefix
}

func New(trusted []netip.Prefix) Resolver { return Resolver{trusted: trusted} }

// FromRequest devolve o IP do cliente, sem porta.
func (r Resolver) FromRequest(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	remote, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	remote = remote.Unmap()
	if cf := req.Header.Get("CF-Connecting-IP"); cf != "" && r.isTrusted(remote) {
		if ip, err := netip.ParseAddr(strings.TrimSpace(cf)); err == nil {
			return ip.Unmap().String()
		}
	}
	return remote.String()
}

// LimitKey é a chave para limites por IP. Agrupa IPv6 por /64: um cliente
// costuma ter o /64 inteiro e trocaria de endereço a cada tentativa.
func (r Resolver) LimitKey(req *http.Request) (string, error) {
	s := r.FromRequest(req)
	ip, err := netip.ParseAddr(s)
	if err != nil || ip.Is4() {
		return s, nil
	}
	p, err := ip.Prefix(64)
	if err != nil {
		return ip.String(), nil
	}
	return p.String(), nil
}

func (r Resolver) isTrusted(a netip.Addr) bool {
	for _, p := range r.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
