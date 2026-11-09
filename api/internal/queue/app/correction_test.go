package app_test

import (
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/queue/app"
	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
)

func TestListQueueShowsCorrectionsOfTheCurrentVersion(t *testing.T) {
	e := newEnv()
	cur := &app.Version{ID: "v-cur", Seq: 2, Label: "Substitutivo", CreatedAt: t0}
	e.w.Described["ready"] = app.Proposition{ID: "ready", Name: "PEC 9/2025", Sphere: domain.SphereFederal, Ready: true, Current: cur}
	e.w.Described["reviewing"] = app.Proposition{ID: "reviewing", Name: "PL 2/2026", Sphere: domain.SphereFederal, InReview: true, Current: cur}
	e.w.Corrections = []app.PendingCorrection{
		// Duas Correções a conferir na mesma Proposição: um item, desde a
		// mais antiga.
		{PropositionID: "ready", VersionID: "v-cur", Reason: domain.ReasonMinorFixCheck, Since: t0.Add(-time.Hour)},
		{PropositionID: "ready", VersionID: "v-cur", Reason: domain.ReasonMinorFixCheck, Since: t0.Add(-3 * time.Hour)},
		{PropositionID: "ready", VersionID: "v-cur", Reason: domain.ReasonErratum, Since: t0.Add(-2 * time.Hour)},
		// De uma Versão já substituída: o Leitor não vê mais nada dela.
		{PropositionID: "ready", VersionID: "v-old", Reason: domain.ReasonErratum, Since: t0.Add(-100 * time.Hour)},
		// Proposição que voltou para "em revisão": fica para depois.
		{PropositionID: "reviewing", VersionID: "v-cur", Reason: domain.ReasonMinorFixCheck, Since: t0},
	}
	items, err := app.NewListQueue(e.w.Props(), e.w.Reviews(), e.w.Contribs(), e.w.Claims(), e.w.Directory(), e.clk).Execute(ana, app.ListQueueInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Reason != domain.ReasonMinorFixCheck || !items[0].Since.Equal(t0.Add(-3*time.Hour)) || items[0].Version.ID != "v-cur" {
		t.Errorf("minor fix item: %+v", items[0])
	}
	if items[1].Reason != domain.ReasonErratum || items[1].PropositionID != "ready" || len(items[1].Missing) != 0 {
		t.Errorf("erratum item: %+v", items[1])
	}
}
