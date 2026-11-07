package clientip

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func resolver() Resolver {
	return New([]netip.Prefix{netip.MustParsePrefix("172.16.0.0/12")})
}

func TestFromRequest(t *testing.T) {
	cases := []struct {
		name, remote, cf, want string
	}{
		{"trusted proxy uses CF header", "172.21.0.1:5000", "203.0.113.7", "203.0.113.7"},
		{"untrusted peer cannot spoof", "198.51.100.9:5000", "203.0.113.7", "198.51.100.9"},
		{"no header falls back to peer", "172.21.0.1:5000", "", "172.21.0.1"},
		{"garbage header ignored", "172.21.0.1:5000", "not-an-ip", "172.21.0.1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.remote
			if c.cf != "" {
				r.Header.Set("CF-Connecting-IP", c.cf)
			}
			if got := resolver().FromRequest(r); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestLimitKeyGroupsIPv6By64(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.21.0.1:5000"
	r.Header.Set("CF-Connecting-IP", "2001:db8:1:2:aaaa::1")
	got, _ := resolver().LimitKey(r)
	if got != "2001:db8:1:2::/64" {
		t.Errorf("got %q", got)
	}
}
