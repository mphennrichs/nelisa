package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
	"github.com/mphennrichs/nelisa/api/internal/review/store"
)

const (
	e1 = "00000000-0000-4000-8000-0000000000c1"
	e2 = "00000000-0000-4000-8000-0000000000c2"
	f1 = "00000000-0000-4000-8000-0000000000d1"
)

func TestMinorFixesRoundTripAndCheckOnce(t *testing.T) {
	pool, _ := testdb.New(t)
	seed(t, pool)
	ctx := context.Background()
	s := store.NewMinorFixes(pool)
	f := domain.MinorFix{
		ID: f1, PropositionID: p1, VersionID: v1, Fields: []string{"summary"},
		Before:    domain.Snapshot{Summary: "Antes", ShortSummary: "C", TopicIDs: []string{}},
		After:     domain.Snapshot{Summary: "Depois", ShortSummary: "C", TopicIDs: []string{v2}},
		CreatedBy: adm, CreatedAt: t0,
	}
	if err := s.Create(ctx, f); err != nil {
		t.Fatal(err)
	}
	got, err := s.FindForUpdate(ctx, f1)
	if err != nil || got.Before.Summary != "Antes" || got.After.TopicIDs[0] != v2 || !got.Unchecked() {
		t.Fatalf("round trip: %+v (%v)", got, err)
	}
	if fs, _ := s.ListUncheckedByProposition(ctx, p1); len(fs) != 1 {
		t.Errorf("unchecked by proposition: %+v", fs)
	}
	if _, err := s.FindForUpdate(ctx, "não-é-uuid"); !errors.Is(err, domain.ErrMinorFixNotFound) {
		t.Errorf("bad id: %v", err)
	}
	// O banco também recusa quem corrigiu conferindo.
	self := got
	self.CheckedBy, self.CheckedAt = adm, &t0
	if err := s.SetChecked(ctx, self); err == nil {
		t.Error("fixer checked own fix in the database")
	}
	got.CheckedBy, got.CheckedAt = rev, &t0
	if err := s.SetChecked(ctx, got); err != nil {
		t.Fatal(err)
	}
	if err := s.SetChecked(ctx, got); !errors.Is(err, domain.ErrMinorFixChecked) {
		t.Errorf("check twice: %v", err)
	}
	if fs, _ := s.ListUnchecked(ctx); len(fs) != 0 {
		t.Errorf("unchecked after check: %+v", fs)
	}
}

func TestErrataOnePendingPerVersionAndDecisions(t *testing.T) {
	pool, _ := testdb.New(t)
	seed(t, pool)
	ctx := context.Background()
	s := store.NewErrata(pool)
	pending := func(id string, at time.Time) domain.Erratum {
		return domain.Erratum{
			ID: id, PropositionID: p1, VersionID: v1, Proposed: domain.Snapshot{Summary: "Novo", ShortSummary: "C", TopicIDs: []string{}},
			Said: "dizia", Correct: "correto", BaseRevision: 2, State: domain.ErratumPending, CreatedBy: adm, CreatedAt: at,
		}
	}
	if err := s.Create(ctx, pending(e1, t0)); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, pending(e2, t0)); !errors.Is(err, domain.ErrErratumPending) {
		t.Fatalf("second pending: %v", err)
	}
	if ok, _ := s.HasPending(ctx, v1); !ok {
		t.Error("HasPending")
	}
	e, err := s.FindForUpdate(ctx, e1)
	if err != nil || e.Proposed.Summary != "Novo" || e.BaseRevision != 2 || !e.Pending() {
		t.Fatalf("round trip: %+v (%v)", e, err)
	}
	// O banco recusa quem propôs aprovando.
	self := e
	at := t0.Add(time.Hour)
	self.State, self.DecidedBy, self.DecidedAt = domain.ErratumApproved, adm, &at
	if err := s.Decide(ctx, self); err == nil {
		t.Error("proposer approved in the database")
	}
	e.State, e.DecidedBy, e.DecidedAt = domain.ErratumApproved, rev, &at
	if err := s.Decide(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.Decide(ctx, e); !errors.Is(err, domain.ErrErratumNotPending) {
		t.Errorf("decide twice: %v", err)
	}
	// Decidida, a Versão aceita outra pendente; só a aprovada é pública.
	if err := s.Create(ctx, pending(e2, t0.Add(2*time.Hour))); err != nil {
		t.Fatal(err)
	}
	approved, _ := s.ListApprovedByVersions(ctx, []string{v1, v2})
	if len(approved) != 1 || approved[0].ID != e1 || approved[0].DecidedAt == nil {
		t.Errorf("approved: %+v", approved)
	}
	if ps, _ := s.ListPending(ctx); len(ps) != 1 || ps[0].ID != e2 {
		t.Errorf("pending: %+v", ps)
	}
	if all, _ := s.ListByProposition(ctx, p1); len(all) != 2 || all[0].ID != e2 {
		t.Errorf("by proposition (newest first): %+v", all)
	}
}
