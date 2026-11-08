package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/review/app"
	"github.com/mphennrichs/nelisa/api/internal/review/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func as(id string, role auth.Role) context.Context {
	return auth.WithActor(context.Background(), auth.Actor{UserID: id, Role: role})
}

var (
	ana  = as("ana", auth.RoleEditor)
	rev  = as("rev", auth.RoleReviewer)
	rev2 = as("rev2", auth.RoleReviewer)
	adm  = as("adm", auth.RoleAdmin)
)

type env struct {
	w   *fakes.World
	clk *clock.Fixed
}

func newEnv() *env {
	e := &env{w: fakes.NewWorld(), clk: &clock.Fixed{T: t0}}
	e.w.PutProposition(fakes.Proposition{ID: "p1", Name: "PEC 221/2019", State: "in_review"})
	e.w.PutVersion(fakes.Version{ID: "v1", PropositionID: "p1", Seq: 1, State: "pending", Authors: []string{"ana"}})
	e.w.SetClaim("p1", "ana")
	return e
}

func (e *env) approve() *app.Approve {
	return app.NewApprove(e.w.Approvals(), e.w.Editorial(), e.w.Claims(), e.w.Tx(), e.w.Audit(), e.clk)
}
func (e *env) second() *app.SecondReview {
	return app.NewSecondReview(e.w.Approvals(), e.w.Editorial(), e.w.Tx(), e.w.Audit(), e.clk)
}
func (e *env) reports() *app.Reports { return app.NewReports(e.w.Approvals(), e.w.Editorial()) }

var in = app.ApproveInput{PropositionID: "p1", VersionID: "v1"}

func TestReviewerApprovesWithFourEyes(t *testing.T) {
	e := newEnv()
	a, err := e.approve().Execute(rev, in)
	if err != nil {
		t.Fatal(err)
	}
	if !a.FourEyes || a.ApprovedBy != "rev" || a.Badge() != 2 {
		t.Errorf("approval = %+v", a)
	}
	if e.w.Version("v1").State != "current" || e.w.Proposition("p1").State != "ready" || e.w.ClaimOf("p1") != "" {
		t.Errorf("promotion/claim: %+v %+v %q", e.w.Version("v1"), e.w.Proposition("p1"), e.w.ClaimOf("p1"))
	}
	evs := e.w.Events()
	if len(evs) != 1 || evs[0].Action != app.ActionApprove || !evs[0].InTx || evs[0].Metadata["four_eyes"] != true || evs[0].TargetID != "v1" {
		t.Errorf("audit = %+v", evs)
	}
	// Aprovar de novo: a Versão já não está em análise.
	if _, err := e.approve().Execute(rev2, in); !errors.Is(err, app.ErrVersionNotPending) {
		t.Errorf("approve twice: %v", err)
	}
}

func TestAuthorCannotApprove(t *testing.T) {
	e := newEnv()
	e.w.PutVersion(fakes.Version{ID: "v1", PropositionID: "p1", Seq: 1, State: "pending", Authors: []string{"ana", "rev"}})
	if _, err := e.approve().Execute(rev, in); !errors.Is(err, domain.ErrFourEyes) {
		t.Fatalf("err = %v", err)
	}
	if e.w.Version("v1").State != "pending" || len(e.w.Events()) != 0 {
		t.Error("refused approval changed state")
	}
}

func TestEditorCannotApprove(t *testing.T) {
	e := newEnv()
	if _, err := e.approve().Execute(ana, in); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("editor: %v", err)
	}
	if _, err := e.approve().Execute(context.Background(), in); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("anonymous: %v", err)
	}
}

func TestIncompleteOrUnconfirmedContentIsRefused(t *testing.T) {
	for _, code := range []string{"summary", "needs_confirmation"} {
		e := newEnv()
		e.w.PutVersion(fakes.Version{ID: "v1", PropositionID: "p1", Seq: 1, State: "pending", Authors: []string{"ana"},
			Missing: []domain.Blocker{{Code: code, Message: "falta " + code}}})
		_, err := e.approve().Execute(rev, in)
		var inc *domain.ContentIncompleteError
		if !errors.As(err, &inc) || inc.Missing[0].Code != code {
			t.Errorf("%s: err = %v", code, err)
		}
		if e.w.Version("v1").State != "pending" {
			t.Errorf("%s: promoted", code)
		}
	}
}

func TestApprovalOfContentChangedSinceReadIsRefused(t *testing.T) {
	e := newEnv()
	e.w.PutVersion(fakes.Version{ID: "v1", PropositionID: "p1", Seq: 1, State: "pending", Revision: 3, Authors: []string{"ana"}})
	stale := in
	stale.Revision = 2
	if _, err := e.approve().Execute(rev, stale); !errors.Is(err, domain.ErrStaleContent) {
		t.Fatalf("stale: %v", err)
	}
	if p := e.w.Proposition("p1"); p.State != "in_review" {
		t.Fatalf("approved content the reviewer never saw: %+v", p)
	}
	fresh := in
	fresh.Revision = 3
	if _, err := e.approve().Execute(rev, fresh); err != nil {
		t.Fatal(err)
	}
}

func TestDraftIsNotApproved(t *testing.T) {
	e := newEnv()
	e.w.PutProposition(fakes.Proposition{ID: "p1", State: "draft"})
	if _, err := e.approve().Execute(rev, in); !errors.Is(err, domain.ErrNotInReview) {
		t.Errorf("err = %v", err)
	}
}

func TestAuditFailureUndoesApproval(t *testing.T) {
	e := newEnv()
	e.w.AuditErr = fakes.ErrBoom
	if _, err := e.approve().Execute(rev, in); !errors.Is(err, fakes.ErrBoom) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := e.w.Approval("v1"); ok {
		t.Error("approval kept")
	}
	if e.w.Version("v1").State != "pending" || e.w.Proposition("p1").State != "in_review" || e.w.ClaimOf("p1") != "ana" {
		t.Error("promotion or claim release kept after audit failure")
	}
}

func TestAdminSelfApprovalNeedsSecondReview(t *testing.T) {
	e := newEnv()
	e.w.PutVersion(fakes.Version{ID: "v1", PropositionID: "p1", Seq: 1, State: "pending", Authors: []string{"adm"}})
	a, err := e.approve().Execute(adm, in)
	if err != nil {
		t.Fatal(err)
	}
	if a.FourEyes || a.Badge() != 1 || !a.PendingSecondReview() {
		t.Fatalf("approval = %+v", a)
	}
	if e.w.Events()[0].Metadata["four_eyes"] != false {
		t.Errorf("audit must record the break: %+v", e.w.Events())
	}
	pending, _ := e.reports().PendingSecondReviews(context.Background())
	if len(pending) != 1 || pending[0].VersionID != "v1" {
		t.Errorf("pending second reviews = %+v", pending)
	}
	sin := app.SecondReviewInput{PropositionID: "p1", VersionID: "v1"}
	// O próprio aprovador tenta a segunda: recusado.
	if _, err := e.second().Execute(adm, sin); !errors.Is(err, domain.ErrFourEyes) {
		t.Errorf("approver second review: %v", err)
	}
	if _, err := e.second().Execute(ana, sin); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("editor second review: %v", err)
	}
	e.clk.Advance(time.Hour)
	a, err = e.second().Execute(rev, sin)
	if err != nil || a.Badge() != 2 || a.SecondReviewBy != "rev" {
		t.Fatalf("second review: %+v %v", a, err)
	}
	badges, _ := e.reports().Badges(context.Background(), []string{"v1", "v-x"})
	if badges["v1"] != 2 || len(badges) != 1 {
		t.Errorf("badges = %v", badges)
	}
	if _, err := e.second().Execute(rev2, sin); !errors.Is(err, domain.ErrNoSecondReviewPending) {
		t.Errorf("third: %v", err)
	}
	if evs := e.w.Events(); len(evs) != 2 || evs[1].Action != app.ActionSecondReview || !evs[1].InTx {
		t.Errorf("audit = %+v", evs)
	}
}

func TestSecondReviewOnlyForCurrentVersion(t *testing.T) {
	e := newEnv()
	e.w.PutVersion(fakes.Version{ID: "v1", PropositionID: "p1", Seq: 1, State: "superseded", Authors: []string{"adm"}})
	e.w.PutApproval(domain.Approval{VersionID: "v1", PropositionID: "p1", ApprovedBy: "adm", ApprovedAt: t0})
	if _, err := e.second().Execute(rev, app.SecondReviewInput{PropositionID: "p1", VersionID: "v1"}); !errors.Is(err, domain.ErrNoSecondReviewPending) {
		t.Errorf("superseded: %v", err)
	}
	if _, err := e.second().Execute(rev, app.SecondReviewInput{PropositionID: "p2", VersionID: "v1"}); !errors.Is(err, app.ErrVersionNotFound) {
		t.Errorf("wrong proposition: %v", err)
	}
}

func TestStatusDecidesForTheActor(t *testing.T) {
	e := newEnv()
	r := e.reports()
	s, err := r.Status(rev, "p1")
	if err != nil || !s.Assessment.CanApprove() || s.Current != nil {
		t.Errorf("reviewer: %+v %v", s, err)
	}
	s, _ = r.Status(ana, "p1")
	if s.Assessment.CanApprove() || len(s.Assessment.Blockers) != 1 || s.Assessment.Blockers[0].Code != "role" { // sem a role, só ela
		t.Errorf("author editor: %+v", s.Assessment)
	}
	e.w.PutVersion(fakes.Version{ID: "v1", PropositionID: "p1", Seq: 1, State: "pending", Authors: []string{"adm"}})
	s, _ = r.Status(adm, "p1")
	if !s.Assessment.CanApprove() || !s.Assessment.BreaksFourEyes {
		t.Errorf("admin author: %+v", s.Assessment)
	}
	if _, err := e.approve().Execute(adm, in); err != nil {
		t.Fatal(err)
	}
	s, _ = r.Status(rev, "p1")
	if s.Current == nil || s.Current.Badge() != 1 || !s.CanSecondReview {
		t.Errorf("after admin approval, reviewer: %+v", s)
	}
	s, _ = r.Status(adm, "p1")
	if s.CanSecondReview {
		t.Error("approver cannot second-review")
	}
	if _, err := r.Status(context.Background(), "p1"); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("anonymous: %v", err)
	}
}
