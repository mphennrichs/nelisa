package domain

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseLink(t *testing.T) {
	ok := map[string]string{
		"https://www.camara.leg.br/proposicoesWeb/fichadetramitacao?idProposicao=1": "www.camara.leg.br",
		"  https://G1.Globo.com./noticia  ":                                         "g1.globo.com",
	}
	for in, host := range ok {
		l, err := ParseLink(in)
		if err != nil || l.Host != host {
			t.Errorf("ParseLink(%q) = %+v, %v; want host %q", in, l, err, host)
		}
	}
	bad := []string{
		"", "http://camara.leg.br", "ftp://camara.leg.br", "https://user:pw@camara.leg.br",
		"https://camara.leg.br:8443/x", "https://127.0.0.1/x", "https://[::1]/x", "https://localhost/x",
		"https://exa mple.com", "javascript:alert(1)", "https://" + strings.Repeat("a", MaxLinkLen),
		"https://-bad.com", "https://x.123", "https:camara.leg.br",
	}
	for _, in := range bad {
		if _, err := ParseLink(in); !errors.Is(err, ErrInvalidLink) {
			t.Errorf("ParseLink(%q) err = %v, want ErrInvalidLink", in, err)
		}
	}
}

func TestIsOfficial(t *testing.T) {
	reg := []string{"camara.sp.gov.br", "cmsp.org"}
	for host, want := range map[string]bool{
		"www.camara.leg.br": true, "leg.br": true, "www.al.sp.gov.br": true,
		"cmsp.org": true, "www.cmsp.org": true, "evilcmsp.org": false,
		"leg.br.evil.com": false, "g1.globo.com": false,
	} {
		if got := IsOfficial(host, reg); got != want {
			t.Errorf("IsOfficial(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestTrackingCode(t *testing.T) {
	c, err := NewTrackingCode(rand.Reader)
	if err != nil || len(c) != TrackingCodeLen {
		t.Fatalf("NewTrackingCode = %q, %v", c, err)
	}
	disp := c.Display()
	got, err := ParseTrackingCode(strings.ToLower(disp))
	if err != nil || got != c {
		t.Fatalf("ParseTrackingCode(%q) = %q, %v; want %q", disp, got, err, c)
	}
	// O e I/L são lidos como 0 e 1.
	got, err = ParseTrackingCode("oil0-1234-abcd")
	if err != nil || got != "01101234ABCD" {
		t.Errorf("ParseTrackingCode crockford = %q, %v", got, err)
	}
	for _, bad := range []string{"", "ABCD", "ABCD-EFGH-JKMN-P", "ABCD-EFGH-JKMU", strings.Repeat("A", 100)} {
		if _, err := ParseTrackingCode(bad); !errors.Is(err, ErrInvalidTrackingCode) {
			t.Errorf("ParseTrackingCode(%q) err = %v", bad, err)
		}
	}
	// Determinístico com a mesma fonte; hash estável.
	a, _ := NewTrackingCode(bytes.NewReader(make([]byte, 12)))
	if a != "000000000000" || a.Hash() != TrackingCode("000000000000").Hash() || len(a.Hash()) != 64 {
		t.Errorf("código zerado = %q", a)
	}
	if _, err := NewTrackingCode(bytes.NewReader(nil)); err == nil {
		t.Error("NewTrackingCode sem entropia deveria falhar")
	}
}

func TestNewContribution(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	code := TrackingCode("ABCDEFGHJKMN")
	prop := &PropositionRef{ID: "p1", Sphere: SphereFederal}
	ctx := Context{Proposition: prop, Registered: []string{"cmsp.org"}, Now: now}

	c, err := NewContribution("c1", code, Submission{
		Kind: KindIndication, Link: "https://www.camara.leg.br/x", Sphere: SphereFederal, Note: "  veja  ",
	}, ctx)
	if err != nil || c.State != StatePending || c.Payload.Note != "veja" || c.TrackingHash != code.Hash() || c.PropositionID != "" {
		t.Fatalf("indicação = %+v, %v", c, err)
	}
	if _, err := NewContribution("c", code, Submission{Kind: KindIndication, Link: "https://g1.globo.com/x", Sphere: SphereFederal}, ctx); !errors.Is(err, ErrNotOfficialSource) {
		t.Errorf("indicação fora de fonte oficial: %v", err)
	}
	if _, err := NewContribution("c", code, Submission{Kind: KindIndication, Link: "https://cmsp.org/x", Sphere: "x"}, ctx); !errors.Is(err, ErrInvalidSphere) {
		t.Errorf("indicação esfera inválida: %v", err)
	}

	up := Submission{Kind: KindProgressUpdate, Link: "https://g1.globo.com/n", EventType: EventVotePostponed, EventDate: "2026-11-20", Sphere: SphereMunicipal}
	c, err = NewContribution("c2", code, up, ctx)
	if err != nil || c.PropositionID != "p1" || c.Sphere != SphereFederal || c.Payload.EventDate != "2026-11-20" {
		t.Fatalf("atualização = %+v, %v", c, err)
	}
	if _, err := NewContribution("c", code, up, Context{Now: now}); !errors.Is(err, ErrPropositionNotFound) {
		t.Errorf("atualização sem proposição: %v", err)
	}
	for _, d := range []string{"2028-01-01", "1980-01-01", "20/11/2026", ""} {
		u := up
		u.EventDate = d
		if _, err := NewContribution("c", code, u, ctx); !errors.Is(err, ErrInvalidEventDate) {
			t.Errorf("data %q: %v", d, err)
		}
	}
	u := up
	u.EventType = "x"
	if _, err := NewContribution("c", code, u, ctx); !errors.Is(err, ErrInvalidEventType) {
		t.Errorf("tipo de evento: %v", err)
	}

	req := Submission{Kind: KindSourceRequest, Domain: "CamaraMunicipal.Exemplo.org.", Link: "https://camaramunicipal.exemplo.org/sobre", SourceName: " Câmara  de Exemplo ", Sphere: SphereMunicipal}
	c, err = NewContribution("c3", code, req, ctx)
	if err != nil || c.Payload.Domain != "camaramunicipal.exemplo.org" || c.Payload.SourceName != "Câmara de Exemplo" {
		t.Fatalf("pedido = %+v, %v", c, err)
	}
	for _, d := range []string{"www.al.sp.gov.br", "www.cmsp.org"} {
		r := req
		r.Domain = d
		if _, err := NewContribution("c", code, r, ctx); !errors.Is(err, ErrOfficialSourceExists) {
			t.Errorf("pedido de %q já oficial: %v", d, err)
		}
	}
	r := req
	r.SourceName = ""
	if _, err := NewContribution("c", code, r, ctx); !errors.Is(err, ErrInvalidSourceName) {
		t.Errorf("pedido sem nome: %v", err)
	}

	if _, err := NewContribution("c", code, Submission{Kind: "x"}, ctx); !errors.Is(err, ErrInvalidKind) {
		t.Errorf("tipo inválido: %v", err)
	}
	long := Submission{Kind: KindIndication, Link: "https://camara.leg.br", Sphere: SphereFederal, Note: strings.Repeat("é", MaxNoteLen+1)}
	if _, err := NewContribution("c", code, long, ctx); !errors.Is(err, ErrNoteTooLong) {
		t.Errorf("nota longa: %v", err)
	}
}

func TestNewOfficialSource(t *testing.T) {
	now := time.Now()
	s, err := NewOfficialSource("id", "WWW.CMSP.ORG", " Câmara Municipal de SP ", SphereMunicipal, "u1", now)
	if err != nil || s.Domain != "www.cmsp.org" || s.Name != "Câmara Municipal de SP" {
		t.Fatalf("NewOfficialSource = %+v, %v", s, err)
	}
	if _, err := NewOfficialSource("id", "https://cmsp.org", "x", SphereMunicipal, "u1", now); !errors.Is(err, ErrInvalidDomain) {
		t.Errorf("domínio com esquema: %v", err)
	}
	if _, err := NewOfficialSource("id", "cmsp.org", "x", "y", "u1", now); !errors.Is(err, ErrInvalidSphere) {
		t.Errorf("esfera: %v", err)
	}
}
