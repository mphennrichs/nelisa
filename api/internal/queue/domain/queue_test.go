package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func TestClaimExpiresAfterThreeDaysWithoutActivity(t *testing.T) {
	c := &domain.Claim{PropositionID: "p", UserID: "ana", ClaimedAt: t0, LastActivityAt: t0}
	if !c.Valid(t0.Add(72*time.Hour-time.Second), true) {
		t.Error("still valid just before 3 days")
	}
	if c.Valid(t0.Add(72*time.Hour), true) {
		t.Error("expired at 3 days")
	}
	if c.Valid(t0, false) {
		t.Error("inactive account holds nothing")
	}
	var none *domain.Claim
	if none.Valid(t0, true) {
		t.Error("nil claim is free")
	}
}

func TestTake(t *testing.T) {
	ana := &domain.Claim{PropositionID: "p", UserID: "ana", ClaimedAt: t0, LastActivityAt: t0}
	_, err := domain.Take(ana, true, "p", "bia", t0)
	var taken *domain.ClaimTakenError
	if !errors.Is(err, domain.ErrClaimTaken) || !errors.As(err, &taken) || taken.HolderID != "ana" {
		t.Errorf("taken: %v", err)
	}
	renewed, err := domain.Take(ana, true, "p", "ana", t0.Add(time.Hour))
	if err != nil || !renewed.ClaimedAt.Equal(t0) || !renewed.LastActivityAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("renew: %+v %v", renewed, err)
	}
	// Expirada (ou de conta inativa): quem chega assume.
	got, err := domain.Take(ana, false, "p", "bia", t0.Add(80*time.Hour))
	if err != nil || got.UserID != "bia" || !got.ClaimedAt.Equal(t0.Add(80*time.Hour)) {
		t.Errorf("expired: %+v %v", got, err)
	}
	got, err = domain.Take(nil, false, "p", "bia", t0)
	if err != nil || got.UserID != "bia" || got.PropositionID != "p" {
		t.Errorf("free: %+v %v", got, err)
	}
}

func TestSortBySphereThenOldest(t *testing.T) {
	vote := t0.Add(48 * time.Hour)
	items := []domain.Item{
		{PropositionID: "mun-old", Sphere: domain.SphereMunicipal, Since: t0.Add(-100 * time.Hour)},
		{PropositionID: "fed-new", Sphere: domain.SphereFederal, Since: t0},
		{PropositionID: "est", Sphere: domain.SphereState, Since: t0.Add(-200 * time.Hour)},
		{PropositionID: "fed-old", Sphere: domain.SphereFederal, Since: t0.Add(-time.Hour)},
		{PropositionID: "fed-vote", Sphere: domain.SphereFederal, Since: t0.Add(time.Hour), VoteAt: &vote},
	}
	domain.Sort(items, func(i domain.Item) domain.Item { return i })
	want := []string{"fed-vote", "fed-old", "fed-new", "est", "mun-old"}
	for i, id := range want {
		if items[i].PropositionID != id {
			t.Fatalf("order = %+v, want %v", items, want)
		}
	}
}

func TestParseSphere(t *testing.T) {
	for _, s := range []string{"federal", "state", "municipal"} {
		if _, err := domain.ParseSphere(s); err != nil {
			t.Error(s, err)
		}
	}
	if _, err := domain.ParseSphere("estadual"); !errors.Is(err, domain.ErrInvalidSphere) {
		t.Error(err)
	}
}
