package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/queue/app"
	"github.com/mphennrichs/nelisa/api/internal/queue/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func as(id string, role auth.Role) context.Context {
	return auth.WithActor(context.Background(), auth.Actor{UserID: id, Role: role})
}

var (
	ana = as("ana", auth.RoleEditor)
	bia = as("bia", auth.RoleEditor)
	rev = as("rev", auth.RoleReviewer)
)

type env struct {
	w   *fakes.World
	clk *clock.Fixed
}

func newEnv() *env {
	w := fakes.NewWorld()
	w.People = map[string]app.Person{
		"ana": {Name: "Ana", Active: true}, "bia": {Name: "Bia", Active: true},
		"rev": {Name: "Rui", Active: true}, "ex": {Name: "Ex-membro", Active: false},
	}
	w.Described["p1"] = app.Proposition{ID: "p1", Name: "PEC 1/2020", Sphere: domain.SphereFederal}
	return &env{w: w, clk: &clock.Fixed{T: t0}}
}

func (e *env) claim() *app.Claim {
	return app.NewClaim(e.w.Claims(), e.w.Props(), e.w.Directory(), e.w.Tx(), e.clk)
}
func (e *env) release() *app.Release {
	return app.NewRelease(e.w.Claims(), e.w.Directory(), e.w.Tx(), e.w.Audit(), e.clk)
}
func (e *env) gate() *app.Gate { return app.NewGate(e.w.Claims(), e.w.Directory(), e.clk) }

func TestClaimAndClaimTaken(t *testing.T) {
	e := newEnv()
	c, err := e.claim().Execute(ana, app.ClaimInput{PropositionID: "p1"})
	if err != nil || c.UserID != "ana" || c.HolderName != "Ana" || !c.ClaimedAt.Equal(t0) {
		t.Fatalf("claim: %+v %v", c, err)
	}
	_, err = e.claim().Execute(bia, app.ClaimInput{PropositionID: "p1"})
	var taken *domain.ClaimTakenError
	if !errors.As(err, &taken) || taken.HolderName != "Ana" {
		t.Fatalf("taken: %v", err)
	}
	// Assumir de novo só renova.
	e.clk.Advance(time.Hour)
	c, _ = e.claim().Execute(ana, app.ClaimInput{PropositionID: "p1"})
	if !c.ClaimedAt.Equal(t0) || !c.LastActivityAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("renew: %+v", c)
	}
	if _, err := e.claim().Execute(ana, app.ClaimInput{PropositionID: "nao-existe"}); !errors.Is(err, app.ErrPropositionNotFound) {
		t.Errorf("unknown: %v", err)
	}
	if _, err := e.claim().Execute(context.Background(), app.ClaimInput{PropositionID: "p1"}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("anonymous: %v", err)
	}
	if e.w.Locks == 0 {
		t.Error("claim must lock")
	}
}

func TestTouchForEdit(t *testing.T) {
	e := newEnv()
	g := e.gate()
	ctx := context.Background()
	if err := g.TouchForEdit(ctx, "p1", "ana"); err != nil {
		t.Fatal(err)
	}
	// Segunda pessoa grava com a Posse alheia: recusado.
	if err := g.TouchForEdit(ctx, "p1", "bia"); !errors.Is(err, domain.ErrClaimTaken) {
		t.Fatalf("taken: %v", err)
	}
	// Cada gravação renova a atividade.
	e.clk.Advance(71 * time.Hour)
	if err := g.TouchForEdit(ctx, "p1", "ana"); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(71 * time.Hour)
	if err := g.TouchForEdit(ctx, "p1", "bia"); !errors.Is(err, domain.ErrClaimTaken) {
		t.Errorf("activity must renew: %v", err)
	}
	// Com a Posse expirada, assume.
	e.clk.Advance(2 * time.Hour)
	if err := g.TouchForEdit(ctx, "p1", "bia"); err != nil {
		t.Fatalf("expired: %v", err)
	}
	if c, _ := e.w.Claim("p1"); c.UserID != "bia" || !c.ClaimedAt.Equal(e.clk.T) {
		t.Errorf("after expiry: %+v", c)
	}
	cur, _ := g.Current(ctx, "p1")
	if cur == nil || cur.HolderName != "Bia" {
		t.Errorf("current: %+v", cur)
	}
}

func TestDeactivatedAccountLosesClaims(t *testing.T) {
	e := newEnv()
	e.w.PutClaim(domain.Claim{PropositionID: "p1", UserID: "ex", ClaimedAt: t0, LastActivityAt: t0})
	g := e.gate()
	if cur, _ := g.Current(context.Background(), "p1"); cur != nil {
		t.Errorf("inactive holder still shown: %+v", cur)
	}
	if err := g.TouchForEdit(context.Background(), "p1", "ana"); err != nil {
		t.Errorf("inactive holder blocks edit: %v", err)
	}
	// Conta apagada/desconhecida vale o mesmo.
	e.w.PutClaim(domain.Claim{PropositionID: "p1", UserID: "sumiu", ClaimedAt: t0, LastActivityAt: t0})
	if _, err := e.claim().Execute(bia, app.ClaimInput{PropositionID: "p1"}); err != nil {
		t.Errorf("unknown holder blocks claim: %v", err)
	}
}

func TestRelease(t *testing.T) {
	e := newEnv()
	if _, err := e.claim().Execute(ana, app.ClaimInput{PropositionID: "p1"}); err != nil {
		t.Fatal(err)
	}
	// Editor não solta a Posse de outra pessoa.
	if err := e.release().Execute(bia, app.ReleaseInput{PropositionID: "p1"}); !errors.Is(err, domain.ErrNotHolder) {
		t.Fatalf("editor releases other's: %v", err)
	}
	// A própria: sem Auditoria.
	if err := e.release().Execute(ana, app.ReleaseInput{PropositionID: "p1"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.w.Claim("p1"); ok || len(e.w.Events()) != 0 {
		t.Errorf("own release: %+v", e.w.Events())
	}
	// Revisor solta a de outra pessoa: com Auditoria.
	if _, err := e.claim().Execute(ana, app.ClaimInput{PropositionID: "p1"}); err != nil {
		t.Fatal(err)
	}
	if err := e.release().Execute(rev, app.ReleaseInput{PropositionID: "p1"}); err != nil {
		t.Fatal(err)
	}
	evs := e.w.Events()
	if len(evs) != 1 || evs[0].Action != app.ActionClaimRelease || !evs[0].InTx || evs[0].Metadata["holder_id"] != "ana" {
		t.Errorf("audit = %+v", evs)
	}
	// Soltar o que não existe não é erro.
	if err := e.release().Execute(bia, app.ReleaseInput{PropositionID: "p1"}); err != nil {
		t.Errorf("release free: %v", err)
	}
	// Falha na Auditoria desfaz a soltura.
	if _, err := e.claim().Execute(ana, app.ClaimInput{PropositionID: "p1"}); err != nil {
		t.Fatal(err)
	}
	e.w.AuditErr = fakes.ErrBoom
	if err := e.release().Execute(rev, app.ReleaseInput{PropositionID: "p1"}); !errors.Is(err, fakes.ErrBoom) {
		t.Fatalf("err = %v", err)
	}
	if c, ok := e.w.Claim("p1"); !ok || c.UserID != "ana" {
		t.Error("release kept after audit failure")
	}
	// Posse expirada de outra pessoa: o Editor só limpa.
	e.w.AuditErr = nil
	e.clk.Advance(73 * time.Hour)
	if err := e.release().Execute(bia, app.ReleaseInput{PropositionID: "p1"}); err != nil {
		t.Errorf("expired release: %v", err)
	}
}

func TestListQueueOrderFilterAndSecondReview(t *testing.T) {
	e := newEnv()
	v := func(id string, at time.Time) *app.Version {
		return &app.Version{ID: id, Seq: 1, Label: "Texto original", CreatedAt: at}
	}
	e.w.InReview = []app.Proposition{
		{ID: "mun", Name: "PL 1/2026", Sphere: domain.SphereMunicipal, InReview: true, Pending: v("v-mun", t0.Add(-500*time.Hour))},
		{ID: "fed-new", Name: "PL 2/2026", Sphere: domain.SphereFederal, InReview: true, Pending: v("v-fn", t0.Add(-time.Hour)),
			Missing: []app.Missing{{Code: "summary", Message: "Escreva o Resumo."}}},
		{ID: "fed-old", Name: "PEC 3/2026", Sphere: domain.SphereFederal, InReview: true, Pending: v("v-fo", t0.Add(-10*time.Hour))},
		{ID: "no-pending", Name: "X", Sphere: domain.SphereFederal, InReview: true},
	}
	e.w.Described["ready"] = app.Proposition{ID: "ready", Name: "PEC 9/2025", Sphere: domain.SphereState, Ready: true, Current: v("v-ready", t0)}
	e.w.Described["moved-on"] = app.Proposition{ID: "moved-on", Name: "PEC 8/2025", Sphere: domain.SphereFederal, Ready: true, Current: v("v-newer", t0)}
	e.w.SecondReviews = []app.PendingSecondReview{
		{PropositionID: "ready", VersionID: "v-ready", ApprovedAt: t0.Add(-2 * time.Hour)},
		{PropositionID: "moved-on", VersionID: "v-old", ApprovedAt: t0.Add(-900 * time.Hour)}, // Versão já substituída
	}
	e.w.PutClaim(domain.Claim{PropositionID: "fed-old", UserID: "ana", ClaimedAt: t0, LastActivityAt: t0})
	e.w.PutClaim(domain.Claim{PropositionID: "fed-new", UserID: "ex", ClaimedAt: t0, LastActivityAt: t0})
	// Contribuições pendentes entram na mesma ordem, sem Posse: a
	// Atualização herda a Esfera (e o nome) da Proposição.
	e.w.Described["fed-old"] = e.w.InReview[2]
	e.w.Contributions = []app.PendingContribution{
		{ID: "c-upd", Kind: "progress_update", Sphere: domain.SphereFederal, PropositionID: "fed-old", Subject: "g1.globo.com", SubmittedAt: t0.Add(-5 * time.Hour)},
		{ID: "c-ind", Kind: "indication", Sphere: domain.SphereMunicipal, Subject: "www.cmsp.example.org", SubmittedAt: t0.Add(-900 * time.Hour)},
	}

	list := app.NewListQueue(e.w.Props(), e.w.Reviews(), e.w.Contribs(), e.w.Claims(), e.w.Directory(), e.clk)
	items, err := list.Execute(ana, app.ListQueueInput{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range items {
		ids = append(ids, it.PropositionID+"/"+string(it.Reason))
	}
	want := []string{"fed-old/review", "fed-old/contribution", "fed-new/review", "ready/second_review", "/contribution", "mun/review"}
	if len(ids) != len(want) {
		t.Fatalf("items = %v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order = %v, want %v", ids, want)
		}
	}
	if items[0].Claim == nil || items[0].Claim.HolderName != "Ana" {
		t.Errorf("claim on item: %+v", items[0].Claim)
	}
	if items[2].Claim != nil || len(items[2].Missing) != 1 {
		t.Errorf("inactive holder must look free; missing listed: %+v", items[2])
	}
	if !items[3].Since.Equal(t0.Add(-2*time.Hour)) || items[3].Version.ID != "v-ready" {
		t.Errorf("second review item: %+v", items[3])
	}
	if c := items[1]; c.Claim != nil || c.Contribution == nil || c.Contribution.ID != "c-upd" || c.Name != "PEC 3/2026" {
		t.Errorf("contribution item: %+v", c)
	}
	if c := items[4]; c.Contribution == nil || c.Name != "www.cmsp.example.org" || c.Contribution.Kind != "indication" {
		t.Errorf("indication item: %+v", c)
	}
	items, _ = list.Execute(ana, app.ListQueueInput{Sphere: "municipal"})
	if len(items) != 2 || items[0].PropositionID != "" || items[1].PropositionID != "mun" {
		t.Errorf("filter: %+v", items)
	}
	if _, err := list.Execute(ana, app.ListQueueInput{Sphere: "galactica"}); !errors.Is(err, domain.ErrInvalidSphere) {
		t.Errorf("bad sphere: %v", err)
	}
	if _, err := list.Execute(context.Background(), app.ListQueueInput{}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Errorf("anonymous: %v", err)
	}
}
