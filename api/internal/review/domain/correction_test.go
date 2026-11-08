package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

// As mesmas pessoas de review_test.go: ed (Editor), rev (Revisor), adm
// (Admin).
var now = t0

func target() domain.Target {
	return domain.Target{
		PropositionID: "p1", Ready: true, VersionID: "v1", Seq: 1, Revision: 3,
		Content: domain.Snapshot{Summary: "Resumo", ShortSummary: "Curto", TopicIDs: []string{"t1"}},
	}
}

func TestTargetAcceptsCorrectionsOnlyWhenReadyAndNoPendingErratum(t *testing.T) {
	tg := target()
	if err := tg.CanCorrect(false); err != nil {
		t.Fatal(err)
	}
	if err := tg.CanCorrect(true); !errors.Is(err, domain.ErrErratumPending) {
		t.Errorf("pending erratum: %v", err)
	}
	tg.Ready = false
	if err := tg.CanCorrect(false); !errors.Is(err, domain.ErrNotReady) {
		t.Errorf("not ready: %v", err)
	}
}

func TestMinorFixNeedsAChangeAndAnotherReviewerToCheck(t *testing.T) {
	if _, err := domain.NewMinorFix("f1", target(), nil, domain.Snapshot{}, "ed", now); !errors.Is(err, domain.ErrNoChanges) {
		t.Fatalf("no fields: %v", err)
	}
	after := domain.Snapshot{Summary: "Resumo.", ShortSummary: "Curto", TopicIDs: []string{"t1"}}
	f, err := domain.NewMinorFix("f1", target(), []string{"summary"}, after, "adm", now)
	if err != nil {
		t.Fatal(err)
	}
	if f.Before.Summary != "Resumo" || f.After.Summary != "Resumo." || !f.Unchecked() {
		t.Fatalf("fix: %+v", f)
	}
	for _, tc := range []struct {
		name string
		who  domain.Approver
		want error
	}{
		{"editor", editor, domain.ErrNotReviewer},
		{"the admin who fixed", admin, domain.ErrFourEyes},
	} {
		if err := f.CanCheck(tc.who); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.want)
		}
	}
	if err := f.Check(reviewer, now); err != nil {
		t.Fatal(err)
	}
	if f.CheckedBy != "rev" || f.CheckedAt == nil || f.Unchecked() {
		t.Errorf("checked: %+v", f)
	}
	if err := f.Check(domain.Approver{UserID: "rui", CanReview: true}, now); !errors.Is(err, domain.ErrMinorFixChecked) {
		t.Errorf("check twice: %v", err)
	}
}

func TestNewErratumValidatesTheNoteAndRequiresAChange(t *testing.T) {
	proposed := domain.Snapshot{Summary: "Outro", ShortSummary: "Curto", TopicIDs: []string{"t1"}}
	long := strings.Repeat("á", domain.MaxErratumNoteLen+1)
	for _, tc := range []struct {
		name, said, correct string
		proposed            domain.Snapshot
		want                error
	}{
		{"empty said", "  ", "y", proposed, domain.ErrInvalidErratumNote},
		{"empty correct", "x", "", proposed, domain.ErrInvalidErratumNote},
		{"too long", long, "y", proposed, domain.ErrInvalidErratumNote},
		{"same content", "x", "y", target().Content, domain.ErrNoChanges},
	} {
		if _, err := domain.NewErratum("e1", target(), tc.proposed, tc.said, tc.correct, "ed", now); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, err, tc.want)
		}
	}
	e, err := domain.NewErratum("e1", target(), proposed, " dizia\r\nX ", strings.Repeat("é", domain.MaxErratumNoteLen), "ed", now)
	if err != nil {
		t.Fatal(err)
	}
	if e.Said != "dizia\nX" || e.BaseRevision != 3 || !e.Pending() || e.CreatedBy != "ed" {
		t.Errorf("erratum: %+v", e)
	}
	if e.Stale(3) || !e.Stale(4) {
		t.Errorf("stale")
	}
}

func pendingErratum(by string) domain.Erratum {
	e, _ := domain.NewErratum("e1", target(), domain.Snapshot{Summary: "Outro", ShortSummary: "Curto", TopicIDs: []string{"t1"}}, "x", "y", by, now)
	return e
}

func TestErratumFourEyesHasNoAdminException(t *testing.T) {
	e := pendingErratum("adm")
	if err := e.Approve(admin, now); !errors.Is(err, domain.ErrFourEyes) {
		t.Errorf("admin proposer approves: %v", err)
	}
	if err := e.Reject(admin, "motivo", now); !errors.Is(err, domain.ErrFourEyes) {
		t.Errorf("admin proposer rejects: %v", err)
	}
	if err := e.Approve(editor, now); !errors.Is(err, domain.ErrNotReviewer) {
		t.Errorf("editor approves: %v", err)
	}
	if err := e.Approve(reviewer, now); err != nil {
		t.Fatal(err)
	}
	if e.State != domain.ErratumApproved || e.DecidedBy != "rev" || e.DecidedAt == nil {
		t.Errorf("approved: %+v", e)
	}
	if err := e.Reject(domain.Approver{UserID: "rui", CanReview: true}, "tarde", now); !errors.Is(err, domain.ErrErratumNotPending) {
		t.Errorf("decide twice: %v", err)
	}
}

func TestErratumRejectNeedsReasonAndWithdrawIsTheProposers(t *testing.T) {
	e := pendingErratum("ed")
	if err := e.Reject(reviewer, " \n ", now); !errors.Is(err, domain.ErrInvalidReason) {
		t.Errorf("empty reason: %v", err)
	}
	if err := e.Reject(reviewer, strings.Repeat("x", domain.MaxReasonLen+1), now); !errors.Is(err, domain.ErrInvalidReason) {
		t.Errorf("long reason: %v", err)
	}
	if !e.Pending() {
		t.Fatal("failed reject changed state")
	}
	if err := e.Withdraw("rev", now); !errors.Is(err, domain.ErrNotErratumProposer) {
		t.Errorf("someone else withdraws: %v", err)
	}
	if err := e.Withdraw("ed", now); err != nil {
		t.Fatal(err)
	}
	if e.State != domain.ErratumWithdrawn || e.DecidedBy != "ed" {
		t.Errorf("withdrawn: %+v", e)
	}
	if err := e.Withdraw("ed", now); !errors.Is(err, domain.ErrErratumNotPending) {
		t.Errorf("withdraw twice: %v", err)
	}

	r := pendingErratum("ed")
	if err := r.Reject(reviewer, "  muda pouco  ", now); err != nil || r.State != domain.ErratumRejected || r.Reason != "muda pouco" {
		t.Errorf("rejected: %+v (%v)", r, err)
	}
}
