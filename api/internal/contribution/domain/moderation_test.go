package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
)

func TestNewProgressEvent(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	e, err := domain.NewProgressEvent(" senado ", "2026-11-20", "  Votação   adiada ", "https://g1.globo.com/x", now)
	if err != nil {
		t.Fatal(err)
	}
	if e.Chamber != "senado" || e.Description != "Votação adiada" || !e.At.Equal(time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("evento = %+v", e)
	}
	cases := map[string]struct {
		chamber, date, text, link string
		want                      error
	}{
		"casa vazia":        {"", "2026-11-20", "x", "https://a.b", domain.ErrInvalidChamber},
		"casa desconhecida": {"alesp", "2026-11-20", "x", "https://a.b", domain.ErrInvalidChamber},
		"data ruim":         {"camara", "20/11/2026", "x", "https://a.b", domain.ErrInvalidEventDate},
		"data longe":        {"camara", "2030-01-01", "x", "https://a.b", domain.ErrInvalidEventDate},
		"sem texto":         {"camara", "2026-11-20", "  ", "https://a.b", domain.ErrInvalidEventText},
		"texto longo":       {"camara", "2026-11-20", strings.Repeat("x", 1001), "https://a.b", domain.ErrInvalidEventText},
		"link http":         {"camara", "2026-11-20", "x", "http://a.b", domain.ErrInvalidLink},
	}
	for name, c := range cases {
		if _, err := domain.NewProgressEvent(c.chamber, c.date, c.text, c.link, now); !errors.Is(err, c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestDecisions(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	c := domain.Contribution{Kind: domain.KindIndication, State: domain.StatePending, Payload: domain.Payload{Link: "https://www.camara.leg.br/x?y=1"}}
	if c.Subject() != "www.camara.leg.br" {
		t.Errorf("subject = %q", c.Subject())
	}
	if err := c.Reject("u", domain.ReasonOther, strings.Repeat("x", 1001), now); !errors.Is(err, domain.ErrInternalNoteLen) {
		t.Errorf("nota longa: %v", err)
	}
	if c.State != domain.StatePending || c.DecidedAt != nil {
		t.Fatal("falha não decide")
	}
	if err := c.Reject("u", domain.ReasonDuplicate, "", now); err != nil || c.PublicReason != "duplicate" || c.DecidedAt == nil {
		t.Fatalf("reject = %+v, %v", c, err)
	}
	if err := c.Accept("u", "", now); !errors.Is(err, domain.ErrNotPending) {
		t.Errorf("aceitar rejeitada: %v", err)
	}
	if err := c.LinkProposition("p"); !errors.Is(err, domain.ErrNotLinkable) {
		t.Errorf("link em rejeitada: %v", err)
	}
	s := domain.Contribution{Kind: domain.KindSourceRequest, State: domain.StatePending, Payload: domain.Payload{Domain: "x.org"}}
	if s.Subject() != "x.org" {
		t.Errorf("subject = %q", s.Subject())
	}
	if err := s.CorrectSphere("galáctica"); !errors.Is(err, domain.ErrInvalidSphere) {
		t.Errorf("esfera: %v", err)
	}
}
