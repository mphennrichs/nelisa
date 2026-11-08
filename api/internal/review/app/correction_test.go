package app_test

import (
	"errors"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/review/app"
	"github.com/mphennrichs/nelisa/api/internal/review/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

// Cenário: a PEC está pronta, com a Versão 1 vigente escrita pela Ana
// (Editora) e aprovada; ninguém tem a Posse.
func newCorrectionEnv() (*env, app.CorrectionDeps) {
	e := &env{w: fakes.NewWorld(), clk: newEnv().clk}
	e.w.PutProposition(fakes.Proposition{ID: "p1", Name: "PEC 221/2019", State: "ready"})
	e.w.PutVersion(fakes.Version{
		ID: "v1", PropositionID: "p1", Seq: 1, State: "current", Revision: 2, Authors: []string{"ana"},
		Content: domain.Snapshot{Summary: "Reduz a jornada.", ShortSummary: "Jornada menor.", TopicIDs: []string{"t1"}},
	})
	e.w.Names["rev"] = "Rita"
	return e, app.CorrectionDeps{
		Fixes: e.w.Fixes(), Errata: e.w.Errata(), Content: e.w.Corrections(), Claims: e.w.ClaimGate(),
		Tx: e.w.Tx(), Audit: e.w.Audit(), Clock: e.clk,
	}
}

func fixInput(summary string, revision int) app.ApplyMinorFixInput {
	return app.ApplyMinorFixInput{
		PropositionID: "p1", VersionID: "v1", Summary: summary, ShortSummary: "Jornada menor.", TopicIDs: []string{"t1"}, Revision: revision,
	}
}

func TestApplyMinorFixWritesContentFixAndAuditTogether(t *testing.T) {
	e, d := newCorrectionEnv()
	f, err := app.NewApplyMinorFix(d).Execute(ana, fixInput("Reduz a jornada semanal.", 2))
	if err != nil {
		t.Fatal(err)
	}
	v := e.w.Version("v1")
	if v.Content.Summary != "Reduz a jornada semanal." || v.Revision != 3 || e.w.Proposition("p1").State != "ready" {
		t.Fatalf("content after fix: %+v", v)
	}
	if f.Before.Summary != "Reduz a jornada." || f.After.Summary != "Reduz a jornada semanal." || len(f.Fields) != 1 || f.CreatedBy != "ana" {
		t.Errorf("fix: %+v", f)
	}
	if e.w.ClaimOf("p1") != "ana" {
		t.Errorf("fix must take the free claim")
	}
	ev := e.w.Events()
	if len(ev) != 1 || ev[0].Action != app.ActionMinorFix || !ev[0].InTx || ev[0].TargetID != "v1" {
		t.Fatalf("audit: %+v", ev)
	}
	for _, v := range ev[0].Metadata {
		if s, ok := v.(string); ok && s == "Reduz a jornada semanal." {
			t.Errorf("audit must not carry the text: %+v", ev[0].Metadata)
		}
	}
}

func TestApplyMinorFixRefusals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(e *env)
		in    app.ApplyMinorFixInput
		want  error
	}{
		{"stale revision", nil, fixInput("Outro.", 1), domain.ErrStaleContent},
		{"no change", nil, fixInput("Reduz a jornada.", 2), domain.ErrNoChanges},
		{"claim taken", func(e *env) { e.w.SetClaim("p1", "rev") }, fixInput("Outro.", 2), app.ErrClaimTaken},
		{"not ready", func(e *env) {
			e.w.PutProposition(fakes.Proposition{ID: "p1", State: "in_review"})
		}, fixInput("Outro.", 2), domain.ErrNotReady},
		{"pending erratum", func(e *env) {
			e.w.PutErratum(domain.Erratum{ID: "e1", PropositionID: "p1", VersionID: "v1", State: domain.ErratumPending, CreatedBy: "rev"})
		}, fixInput("Outro.", 2), domain.ErrErratumPending},
		{"superseded version", func(e *env) {
			v := e.w.Version("v1")
			v.State = "superseded"
			e.w.PutVersion(v)
		}, fixInput("Outro.", 2), domain.ErrVersionNotCurrent},
		{"incomplete content", nil, app.ApplyMinorFixInput{PropositionID: "p1", VersionID: "v1", Summary: "X", Revision: 2}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, d := newCorrectionEnv()
			if tc.setup != nil {
				tc.setup(e)
			}
			before := e.w.Version("v1")
			_, err := app.NewApplyMinorFix(d).Execute(ana, tc.in)
			var invalid *app.InvalidContentError
			if tc.want == nil && !errors.As(err, &invalid) || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if after := e.w.Version("v1"); after.Revision != before.Revision || len(e.w.MinorFixes()) != 0 || len(e.w.Events()) != 0 {
				t.Errorf("refused fix changed something: %+v", after)
			}
		})
	}
	var taken *app.ClaimTakenError
	e, d := newCorrectionEnv()
	e.w.SetClaim("p1", "rev")
	if _, err := app.NewApplyMinorFix(d).Execute(ana, fixInput("Outro.", 2)); !errors.As(err, &taken) || taken.HolderName != "Rita" {
		t.Errorf("claim holder: %v", err)
	}
}

func TestMinorFixRollsBackWhenAuditFails(t *testing.T) {
	e, d := newCorrectionEnv()
	e.w.AuditErr = fakes.ErrBoom
	if _, err := app.NewApplyMinorFix(d).Execute(ana, fixInput("Outro.", 2)); !errors.Is(err, fakes.ErrBoom) {
		t.Fatalf("err = %v", err)
	}
	if v := e.w.Version("v1"); v.Revision != 2 || v.Content.Summary != "Reduz a jornada." || len(e.w.MinorFixes()) != 0 || e.w.ClaimOf("p1") != "" {
		t.Errorf("not rolled back: %+v", v)
	}
}

func TestCheckMinorFixIsForAnotherReviewer(t *testing.T) {
	e, d := newCorrectionEnv()
	f, err := app.NewApplyMinorFix(d).Execute(rev, fixInput("Outro.", 2))
	if err != nil {
		t.Fatal(err)
	}
	check := app.NewCheckMinorFix(d)
	in := app.CheckMinorFixInput{PropositionID: "p1", MinorFixID: f.ID}
	if _, err := check.Execute(rev, in); !errors.Is(err, domain.ErrFourEyes) {
		t.Errorf("fixer checks: %v", err)
	}
	if _, err := check.Execute(ana, in); !errors.Is(err, app.ErrForbidden) {
		t.Errorf("editor checks: %v", err)
	}
	if _, err := check.Execute(rev2, app.CheckMinorFixInput{PropositionID: "p2", MinorFixID: f.ID}); !errors.Is(err, domain.ErrMinorFixNotFound) {
		t.Errorf("other proposition: %v", err)
	}
	e.w.AuditErr = fakes.ErrBoom
	if _, err := check.Execute(rev2, in); !errors.Is(err, fakes.ErrBoom) {
		t.Fatalf("audit failure: %v", err)
	}
	if got, _ := e.w.MinorFix(f.ID); !got.Unchecked() {
		t.Fatal("check not rolled back")
	}
	e.w.AuditErr = nil
	if _, err := check.Execute(rev2, in); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.w.MinorFix(f.ID); got.CheckedBy != "rev2" {
		t.Errorf("checked: %+v", got)
	}
	if fs, _ := e.reports().UncheckedMinorFixes(rev2); len(fs) != 0 {
		t.Errorf("still unchecked: %+v", fs)
	}
}

func proposeInput(summary string, revision int) app.ProposeErratumInput {
	return app.ProposeErratumInput{
		PropositionID: "p1", VersionID: "v1", Summary: summary, ShortSummary: "Jornada maior.", TopicIDs: []string{"t1"},
		Said: "dizia que reduz", Correct: "o correto é que aumenta", Revision: revision,
	}
}

func TestErratumLifecycle(t *testing.T) {
	e, d := newCorrectionEnv()
	propose := app.NewProposeErratum(d)
	er, err := propose.Execute(adm, proposeInput("Aumenta a jornada.", 2))
	if err != nil {
		t.Fatal(err)
	}
	// Pendente: nada muda; não exige Posse.
	if v := e.w.Version("v1"); v.Revision != 2 || v.Content.Summary != "Reduz a jornada." || e.w.ClaimOf("p1") != "" {
		t.Fatalf("pending erratum changed content: %+v", v)
	}
	if _, err := propose.Execute(ana, proposeInput("Outro.", 2)); !errors.Is(err, domain.ErrErratumPending) {
		t.Errorf("second pending: %v", err)
	}
	if _, err := app.NewApplyMinorFix(d).Execute(ana, fixInput("Outro.", 2)); !errors.Is(err, domain.ErrErratumPending) {
		t.Errorf("minor fix with pending erratum: %v", err)
	}
	ref := app.ErratumRef{PropositionID: "p1", VersionID: "v1", ErratumID: er.ID}
	approve := app.NewApproveErratum(d)
	if _, err := approve.Execute(adm, app.ApproveErratumInput{ErratumRef: ref, Revision: 2}); !errors.Is(err, domain.ErrFourEyes) {
		t.Errorf("admin proposer approves: %v", err)
	}
	if _, err := approve.Execute(rev, app.ApproveErratumInput{ErratumRef: ref, Revision: 1}); !errors.Is(err, domain.ErrStaleContent) {
		t.Errorf("approve with old revision: %v", err)
	}
	wrong := ref
	wrong.VersionID = "v9"
	if _, err := approve.Execute(rev, app.ApproveErratumInput{ErratumRef: wrong, Revision: 2}); err == nil {
		t.Errorf("approve under another version must fail")
	}

	s, err := e.reports().Status(rev, "p1")
	if err != nil {
		t.Fatal(err)
	}
	if s.PendingErratum == nil || !s.PendingErratum.CanDecide || s.PendingErratum.CanWithdraw || s.CanMinorFix || s.CanProposeErratum {
		t.Fatalf("status for reviewer: %+v", s)
	}
	if s, _ := e.reports().Status(adm, "p1"); s.PendingErratum.CanDecide || !s.PendingErratum.CanWithdraw {
		t.Errorf("status for proposer: %+v", s.PendingErratum)
	}

	e.w.AuditErr = fakes.ErrBoom
	if _, err := approve.Execute(rev, app.ApproveErratumInput{ErratumRef: ref, Revision: 2}); !errors.Is(err, fakes.ErrBoom) {
		t.Fatalf("audit failure: %v", err)
	}
	if v := e.w.Version("v1"); v.Revision != 2 {
		t.Fatal("approval not rolled back")
	}
	e.w.AuditErr = nil
	if _, err := approve.Execute(rev, app.ApproveErratumInput{ErratumRef: ref, Revision: 2}); err != nil {
		t.Fatal(err)
	}
	v := e.w.Version("v1")
	if v.Content.Summary != "Aumenta a jornada." || v.Revision != 3 || len(v.Authors) != 2 || v.Authors[1] != "adm" {
		t.Fatalf("content after approval: %+v", v)
	}
	got, _ := e.w.Erratum(er.ID)
	if got.State != domain.ErratumApproved || got.DecidedBy != "rev" {
		t.Errorf("erratum: %+v", got)
	}
	pub, _ := e.reports().ApprovedErrata(ana, []string{"v1"})
	if len(pub) != 1 || pub[0].ID != er.ID {
		t.Errorf("public errata: %+v", pub)
	}
	if ev := e.w.Events(); len(ev) != 2 || ev[0].Action != app.ActionErratumPropose || ev[1].Action != app.ActionErratumApprove || !ev[1].InTx {
		t.Errorf("audit: %+v", ev)
	}
}

func TestErratumRejectWithdrawAndStaleBase(t *testing.T) {
	e, d := newCorrectionEnv()
	er, err := app.NewProposeErratum(d).Execute(ana, proposeInput("Aumenta.", 2))
	if err != nil {
		t.Fatal(err)
	}
	ref := app.ErratumRef{PropositionID: "p1", VersionID: "v1", ErratumID: er.ID}
	if _, err := app.NewWithdrawErratum(d).Execute(rev, ref); !errors.Is(err, domain.ErrNotErratumProposer) {
		t.Errorf("someone else withdraws: %v", err)
	}
	if _, err := app.NewRejectErratum(d).Execute(rev, app.RejectErratumInput{ErratumRef: ref, Reason: "não muda o sentido"}); err != nil {
		t.Fatal(err)
	}
	if v := e.w.Version("v1"); v.Revision != 2 {
		t.Errorf("reject changed content")
	}
	if pub, _ := e.reports().ApprovedErrata(ana, []string{"v1"}); len(pub) != 0 {
		t.Errorf("rejected erratum is public: %+v", pub)
	}

	er2, err := app.NewProposeErratum(d).Execute(ana, proposeInput("Aumenta.", 2))
	if err != nil {
		t.Fatal(err)
	}
	ref2 := app.ErratumRef{PropositionID: "p1", VersionID: "v1", ErratumID: er2.ID}
	// O conteúdo vigente mudou por fora depois da proposta.
	v := e.w.Version("v1")
	v.Revision = 5
	e.w.PutVersion(v)
	if s, _ := e.reports().Status(rev, "p1"); s.PendingErratum == nil || !s.PendingErratum.Stale {
		t.Errorf("stale flag: %+v", s.PendingErratum)
	}
	for _, r := range []int{2, 5} {
		if _, err := app.NewApproveErratum(d).Execute(rev, app.ApproveErratumInput{ErratumRef: ref2, Revision: r}); !errors.Is(err, domain.ErrStaleContent) {
			t.Errorf("approve over changed base (revision %d): %v", r, err)
		}
	}
	if _, err := app.NewWithdrawErratum(d).Execute(ana, ref2); err != nil {
		t.Fatal(err)
	}
	s, _ := e.reports().Status(ana, "p1")
	if s.PendingErratum != nil || len(s.Errata) != 2 || !s.CanMinorFix || !s.CanProposeErratum {
		t.Errorf("status after withdraw: %+v", s)
	}
	if ev := e.w.Events(); ev[len(ev)-1].Action != app.ActionErratumWithdraw {
		t.Errorf("audit: %+v", ev)
	}
}
