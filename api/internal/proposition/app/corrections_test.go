package app_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

func TestRewriteCurrentOnlyTouchesTheCurrentVersion(t *testing.T) {
	e := newEnv()
	pid, v := e.readyWithContent(t)
	ed := e.editorial()
	ctx := context.Background()

	s, err := ed.LockCurrent(ctx, pid, v.ID)
	if err != nil || !s.Ready || s.Content.Revision != 1 || s.Content.Summary != "Reduz a jornada." {
		t.Fatalf("lock current: %+v %v", s, err)
	}
	edit, err := ed.CheckCurrentEdit(ctx, domain.ContentEdit{Summary: " Reduz a jornada semanal. ", ShortSummary: "Jornada menor.", TopicIDs: []string{"t-trab"}}, s.Content.TopicIDs)
	if err != nil || edit.Summary != "Reduz a jornada semanal." {
		t.Fatalf("check: %+v %v", edit, err)
	}
	for name, bad := range map[string]domain.ContentEdit{
		"incomplete":     {Summary: "R", TopicIDs: []string{"t-trab"}},
		"inactive topic": {Summary: "R", ShortSummary: "C", TopicIDs: []string{"t-old"}},
	} {
		if _, err := ed.CheckCurrentEdit(ctx, bad, s.Content.TopicIDs); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, _, err := ed.RewriteCurrent(ctx, pid, v.ID, edit, "bia", 0); !errors.Is(err, domain.ErrStaleContent) {
		t.Errorf("stale: %v", err)
	}
	c, fields, err := ed.RewriteCurrent(ctx, pid, v.ID, edit, "bia", 1)
	if err != nil || !slices.Equal(fields, []string{"summary"}) || c.Revision != 2 || !c.HasAuthor("bia") || !c.HasAuthor("ana") {
		t.Fatalf("rewrite: %+v %v %v", c, fields, err)
	}
	if _, fields, err := ed.RewriteCurrent(ctx, pid, v.ID, edit, "bia", 2); err != nil || len(fields) != 0 {
		t.Errorf("no-op rewrite: %v %v", fields, err)
	}
	if got, _ := e.w.ContentOf(v.ID); got.Revision != 2 {
		t.Errorf("no-op must not bump revision: %d", got.Revision)
	}

	// Versão nova: a em análise não é a vigente.
	res, err := e.createVersion().Execute(bia, app.CreateVersionInput{PropositionID: pid, Label: "Substitutivo", OfficialTextURL: "https://x.leg.br/sub"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ed.LockCurrent(ctx, pid, res.Version.ID); !errors.Is(err, domain.ErrVersionNotCurrent) {
		t.Errorf("pending as current: %v", err)
	}
	if s, err := ed.Current(ctx, pid); err != nil || s == nil || s.Ready || s.Version.ID != v.ID {
		t.Errorf("current while in review: %+v %v", s, err)
	}
}

func TestGetPropositionCanMinorFixNeedsReviewAndFreeClaim(t *testing.T) {
	e := newEnv()
	pid, _ := e.readyWithContent(t)
	get := app.NewGetProposition(e.w.Repos(), e.w.Links(), e.w.Timeline(), e.w.Claims(), e.w.Reviews(), e.w.People())
	e.w.Statuses[pid] = app.ReviewStatus{CanMinorFix: true, CanProposeErratum: true}
	if d, _ := get.Execute(ana, app.GetPropositionInput{PropositionID: pid}); !d.Editorial.CanMinorFix {
		t.Error("free claim: can minor fix")
	}
	e.w.SetClaim(pid, &app.ClaimInfo{UserID: "bia", HolderName: "Bia"})
	if d, _ := get.Execute(ana, app.GetPropositionInput{PropositionID: pid}); d.Editorial.CanMinorFix || !d.Editorial.Review.CanProposeErratum {
		t.Errorf("someone else's claim: %+v", d.Editorial)
	}
	e.w.Statuses[pid] = app.ReviewStatus{
		PendingErratum: &app.ErratumInfo{ID: "e1", CreatedBy: "rev", State: "pending"},
		Errata:         []app.ErratumInfo{{ID: "e0", CreatedBy: "adm", DecidedBy: "bia", State: "rejected"}},
		MinorFixes:     []app.MinorFixInfo{{ID: "f1", CreatedBy: "ana"}},
	}
	e.w.Names = map[string]string{"rev": "Rui", "adm": "Ada", "bia": "Bia", "ana": "Ana"}
	d, _ := get.Execute(bia, app.GetPropositionInput{PropositionID: pid})
	if d.Editorial.CanMinorFix || d.Editorial.People["rev"] != "Rui" || d.Editorial.People["adm"] != "Ada" || d.Editorial.People["ana"] != "Ana" {
		t.Errorf("names of who corrected: %+v", d.Editorial.People)
	}
}

func TestPublicDetailCarriesOnlyCurrentErrata(t *testing.T) {
	e := newEnv()
	_, v := e.readyWithContent(t)
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	e.w.Errata = []domain.PublicErratum{
		{VersionID: v.ID, ApprovedAt: at, Said: "dizia X", Correct: "correto Y"},
		{VersionID: "outra", ApprovedAt: at, Said: "SEGREDO", Correct: "z"},
	}
	d, err := app.NewGetPublicProposition(e.w.Repos(), e.w.Links(), e.w.Timeline(), e.w.Reviews()).Execute(context.Background(), "pec-221-2019")
	if err != nil || len(d.Errata) != 1 || d.Errata[0].Said != "dizia X" {
		t.Fatalf("public errata: %+v %v", d.Errata, err)
	}
}
