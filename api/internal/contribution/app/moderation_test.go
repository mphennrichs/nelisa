package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/contribution/app"
	"github.com/mphennrichs/nelisa/api/internal/contribution/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
)

const ana = "u-ana"

type mod struct {
	w      *fakes.World
	get    *app.GetContribution
	accept *app.AcceptContribution
	reject *app.RejectContribution
	sphere *app.CorrectSphere
	link   *app.LinkProposition
	del    *app.DeleteProgressEntry
}

func newMod() mod {
	w := fakes.New()
	w.Team["p1"] = app.TeamProposition{ID: "p1", Name: "PEC 221/2019", Slug: "pec-221-2019", Sphere: domain.SphereFederal}
	d := app.ModerationDeps{
		Contributions: fakes.Contributions{W: w}, Sources: fakes.Sources{W: w}, Props: fakes.Props{W: w},
		Progress: fakes.Progress{W: w}, Tx: fakes.Tx{W: w}, Audit: fakes.Audit{W: w}, Clock: fixedClock{now},
	}
	return mod{
		w: w, get: app.NewGetContribution(d), accept: app.NewAcceptContribution(d), reject: app.NewRejectContribution(d),
		sphere: app.NewCorrectSphere(d), link: app.NewLinkProposition(d), del: app.NewDeleteProgressEntry(d),
	}
}

func (m mod) add(c domain.Contribution) {
	c.State = domain.StatePending
	if c.SubmittedAt.IsZero() {
		c.SubmittedAt = now.Add(-time.Hour)
	}
	m.w.Contributions[c.ID] = c
}

func update() domain.Contribution {
	return domain.Contribution{
		ID: "c1", Kind: domain.KindProgressUpdate, PropositionID: "p1", Sphere: domain.SphereFederal,
		Payload: domain.Payload{Link: "https://g1.globo.com/x", EventType: domain.EventVotePostponed, EventDate: "2026-11-20", Note: "adiada"},
	}
}

func event() *app.ProgressEventInput {
	return &app.ProgressEventInput{Chamber: "camara", Date: "2026-11-20", Description: "Votação adiada no Plenário", SourceURL: "https://g1.globo.com/x"}
}

func TestModerationNeedsEditor(t *testing.T) {
	m := newMod()
	m.add(update())
	for name, ctx := range map[string]context.Context{"anônimo": context.Background(), "leitor": as(auth.Role("reader"), "u")} {
		if _, err := m.get.Execute(ctx, "c1"); err == nil {
			t.Errorf("%s: get deveria falhar", name)
		}
		if _, err := m.accept.Execute(ctx, app.AcceptInput{ID: "c1", Event: event()}); err == nil {
			t.Errorf("%s: accept deveria falhar", name)
		}
	}
	if m.w.Contributions["c1"].State != domain.StatePending || len(m.w.Events) != 0 {
		t.Error("nada muda sem permissão")
	}
}

func TestAcceptProgressUpdateRecordsManualEntry(t *testing.T) {
	m := newMod()
	m.add(update())
	ctx := as(auth.RoleEditor, ana)

	if _, err := m.accept.Execute(ctx, app.AcceptInput{ID: "c1"}); !errors.Is(err, app.ErrEventRequired) {
		t.Errorf("sem evento: %v", err)
	}
	bad := event()
	bad.SourceURL = "http://g1.globo.com/x"
	if _, err := m.accept.Execute(ctx, app.AcceptInput{ID: "c1", Event: bad}); !errors.Is(err, domain.ErrInvalidLink) {
		t.Errorf("link http: %v", err)
	}
	if m.w.Contributions["c1"].State != domain.StatePending || len(m.w.Manual) != 0 {
		t.Fatal("falha não muda nada")
	}

	v, err := m.accept.Execute(ctx, app.AcceptInput{ID: "c1", InternalNote: " conferido ", Event: event()})
	if err != nil {
		t.Fatal(err)
	}
	if v.State != domain.StateAccepted || v.DecidedBy != ana || v.InternalNote != "conferido" || v.Proposition == nil || v.Proposition.Slug != "pec-221-2019" {
		t.Errorf("view = %+v", v)
	}
	e, ok := m.w.Manual["c1"]
	want := time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC) // meio-dia em Brasília
	if !ok || e.PropositionID != "p1" || e.Chamber != "camara" || !e.At.Equal(want) || e.Description != "Votação adiada no Plenário" {
		t.Errorf("evento = %+v", e)
	}
	if len(m.w.Events) != 1 || m.w.Events[0].Action != app.ActionContributionAccept || !m.w.Events[0].InTx || m.w.Events[0].ActorID != ana {
		t.Errorf("auditoria = %+v", m.w.Events)
	}
	// Decidida não se decide de novo.
	if _, err := m.reject.Execute(ctx, app.RejectInput{ID: "c1", Reason: domain.ReasonOther}); !errors.Is(err, domain.ErrNotPending) {
		t.Errorf("rejeitar aceita: %v", err)
	}

	// Apagar o evento manual: auditado; a Contribuição continua aceita.
	if err := m.del.Execute(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.w.Manual["c1"]; ok || m.w.Contributions["c1"].State != domain.StateAccepted {
		t.Error("evento não apagado")
	}
	if last := m.w.Events[len(m.w.Events)-1]; last.Action != app.ActionContributionProgressDelete {
		t.Errorf("auditoria = %+v", last)
	}
	if err := m.del.Execute(ctx, "c1"); !errors.Is(err, domain.ErrNoProgressEntry) {
		t.Errorf("apagar de novo: %v", err)
	}
}

func TestAcceptProgressUpdateOfDeletedProposition(t *testing.T) {
	m := newMod()
	c := update()
	c.PropositionID = "sumiu"
	m.add(c)
	if _, err := m.accept.Execute(as(auth.RoleEditor, ana), app.AcceptInput{ID: "c1", Event: event()}); !errors.Is(err, app.ErrPropositionGone) {
		t.Errorf("err = %v", err)
	}
	if m.w.Contributions["c1"].State != domain.StatePending {
		t.Error("deveria continuar pendente")
	}
}

func TestRejectNeedsReasonAndKeepsNoteInternal(t *testing.T) {
	m := newMod()
	m.add(update())
	ctx := as(auth.RoleReviewer, ana)
	if _, err := m.reject.Execute(ctx, app.RejectInput{ID: "c1", Reason: "porque sim"}); !errors.Is(err, domain.ErrInvalidReason) {
		t.Errorf("motivo fora da lista: %v", err)
	}
	v, err := m.reject.Execute(ctx, app.RejectInput{ID: "c1", Reason: domain.ReasonSourceNotConfirmed, InternalNote: "link fora do ar"})
	if err != nil || v.State != domain.StateRejected || v.PublicReason != "source_not_confirmed" || v.InternalNote != "link fora do ar" {
		t.Fatalf("reject = %+v, %v", v, err)
	}
	if len(m.w.Manual) != 0 {
		t.Error("rejeitar não grava evento")
	}
	if e := m.w.Events[0]; e.Action != app.ActionContributionReject || e.Metadata["reason"] != "source_not_confirmed" {
		t.Errorf("auditoria = %+v", e)
	}
	if _, err := m.reject.Execute(ctx, app.RejectInput{ID: "nada", Reason: domain.ReasonOther}); !errors.Is(err, domain.ErrContributionNotFound) {
		t.Errorf("inexistente: %v", err)
	}
}

func TestAcceptSourceRequestRegistersOfficialSource(t *testing.T) {
	m := newMod()
	m.add(domain.Contribution{
		ID: "c2", Kind: domain.KindSourceRequest, Sphere: domain.SphereState,
		Payload: domain.Payload{Domain: "al.sp.example.org", SourceName: "Assembleia de SP", Link: "https://al.sp.example.org/sobre"},
	})
	ctx := as(auth.RoleEditor, ana)
	if _, err := m.sphere.Execute(ctx, app.CorrectSphereInput{ID: "c2", Sphere: domain.SphereMunicipal}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.accept.Execute(ctx, app.AcceptInput{ID: "c2"}); err != nil {
		t.Fatal(err)
	}
	if len(m.w.Sources) != 1 {
		t.Fatalf("fontes = %+v", m.w.Sources)
	}
	for _, s := range m.w.Sources {
		if s.Domain != "al.sp.example.org" || s.Sphere != domain.SphereMunicipal || s.CreatedBy != ana {
			t.Errorf("fonte = %+v", s)
		}
	}
	var actions []string
	for _, e := range m.w.Events {
		actions = append(actions, e.Action)
	}
	if len(actions) != 3 || actions[0] != app.ActionContributionSphere || actions[1] != app.ActionOfficialSourceCreate || actions[2] != app.ActionContributionAccept {
		t.Errorf("auditoria = %v", actions)
	}

	// Domínio que virou Fonte oficial enquanto o pedido esperava: o aceite
	// falha inteiro, e a equipe rejeita como "já cadastrada".
	m.add(domain.Contribution{
		ID: "c3", Kind: domain.KindSourceRequest, Sphere: domain.SphereState,
		Payload: domain.Payload{Domain: "sub.al.sp.example.org", SourceName: "X", Link: "https://al.sp.example.org/x"},
	})
	if _, err := m.accept.Execute(ctx, app.AcceptInput{ID: "c3"}); !errors.Is(err, domain.ErrOfficialSourceExists) {
		t.Errorf("já coberto: %v", err)
	}
	if m.w.Contributions["c3"].State != domain.StatePending {
		t.Error("deveria continuar pendente")
	}
}

func TestIndicationAcceptThenLink(t *testing.T) {
	m := newMod()
	m.add(domain.Contribution{ID: "c4", Kind: domain.KindIndication, Sphere: domain.SphereFederal, Payload: domain.Payload{Link: "https://www.camara.leg.br/x"}})
	ctx := as(auth.RoleEditor, ana)
	if _, err := m.link.Execute(ctx, app.LinkPropositionInput{ID: "c4", PropositionID: "p1"}); !errors.Is(err, domain.ErrNotLinkable) {
		t.Errorf("link antes de aceitar: %v", err)
	}
	if _, err := m.accept.Execute(ctx, app.AcceptInput{ID: "c4"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.link.Execute(ctx, app.LinkPropositionInput{ID: "c4", PropositionID: "nao-existe"}); !errors.Is(err, app.ErrPropositionGone) {
		t.Errorf("proposição inexistente: %v", err)
	}
	v, err := m.link.Execute(ctx, app.LinkPropositionInput{ID: "c4", PropositionID: "p1"})
	if err != nil || v.CreatedProposition == nil || v.CreatedProposition.ID != "p1" {
		t.Fatalf("link = %+v, %v", v, err)
	}
	if _, err := m.link.Execute(ctx, app.LinkPropositionInput{ID: "c4", PropositionID: "p1"}); !errors.Is(err, domain.ErrNotLinkable) {
		t.Errorf("link de novo: %v", err)
	}
	if _, err := m.sphere.Execute(ctx, app.CorrectSphereInput{ID: "c4", Sphere: domain.SphereState}); !errors.Is(err, domain.ErrNotPending) {
		t.Errorf("esfera depois de decidir: %v", err)
	}
}

func TestSphereOfProgressUpdateIsFixed(t *testing.T) {
	m := newMod()
	m.add(update())
	if _, err := m.sphere.Execute(as(auth.RoleEditor, ana), app.CorrectSphereInput{ID: "c1", Sphere: domain.SphereState}); !errors.Is(err, domain.ErrSphereFixed) {
		t.Errorf("err = %v", err)
	}
}
