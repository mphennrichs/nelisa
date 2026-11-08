package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	"github.com/mphennrichs/nelisa/api/internal/proposition/store"
)

var t0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

const (
	p1 = "00000000-0000-4000-8000-000000000001"
	p2 = "00000000-0000-4000-8000-000000000002"
	p3 = "00000000-0000-4000-8000-000000000003"
	l1 = "00000000-0000-4000-8000-0000000000a1"
	l2 = "00000000-0000-4000-8000-0000000000a2"
	l3 = "00000000-0000-4000-8000-0000000000a3"
	l4 = "00000000-0000-4000-8000-0000000000a4"
	ed = "00000000-0000-4000-8000-0000000000e1"
)

func mkProp(t *testing.T, s *store.Propositions, id, slug string, number int, origin domain.Chamber) domain.Proposition {
	t.Helper()
	presented := t0.Add(-24 * time.Hour)
	p, err := domain.NewProposition(id, slug, "PEC", number, 2019, origin, domain.OfficialData{
		OfficialSummary: "Altera a jornada", Authors: []string{"Dep. Fulana"}, PresentedAt: &presented,
		OfficialTextURL: "https://www.camara.leg.br/texto",
	}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPropositionsRoundTripAndConstraints(t *testing.T) {
	pool, _ := testdb.New(t)
	s := store.NewPropositions(pool)
	ctx := context.Background()
	p := mkProp(t, s, p1, "pec-221-2019", 221, domain.ChamberCamara)

	got, err := s.FindBySlug(ctx, "pec-221-2019")
	if err != nil || got.ID != p1 || got.Name() != "PEC 221/2019" || got.ReviewState != domain.ReviewDraft ||
		len(got.Authors) != 1 || got.PresentedAt == nil || !got.CreatedAt.Equal(t0) {
		t.Fatalf("got %+v (%v)", got, err)
	}
	if ok, _ := s.SlugExists(ctx, "pec-221-2019"); !ok {
		t.Error("slug should exist")
	}

	dupIdentity := p
	dupIdentity.ID, dupIdentity.Slug = p2, "outro"
	if err := s.Create(ctx, dupIdentity); !errors.Is(err, domain.ErrPropositionExists) {
		t.Errorf("same origin/kind/number/year: %v", err)
	}
	dupSlug := p
	dupSlug.ID, dupSlug.OriginChamber = p2, domain.ChamberSenado
	if err := s.Create(ctx, dupSlug); !errors.Is(err, domain.ErrSlugTaken) {
		t.Errorf("same slug: %v", err)
	}
	// Mesma numeração, outra Casa iniciadora: permitido (numeração por Casa).
	mkProp(t, s, p2, "pec-221-2019-senado", 221, domain.ChamberSenado)
	if same, _ := s.FindByName(ctx, "PEC", 221, 2019); len(same) != 2 {
		t.Errorf("FindByName = %d", len(same))
	}

	for _, id := range []string{p3, "nao-e-uuid"} {
		if _, err := s.FindByID(ctx, id); !errors.Is(err, domain.ErrPropositionNotFound) {
			t.Errorf("FindByID(%q): %v", id, err)
		}
	}

	// Update: estado, dados oficiais e Apensação.
	attached := mkProp(t, s, p3, "pec-8-2019", 8, domain.ChamberCamara)
	attached.AttachedToName = "PEC 221/2019"
	attached.AttachTo(p1)
	_ = attached.SubmitForReview(t0.Add(time.Hour))
	attached.Status = "Prejudicada"
	if err := s.Update(ctx, attached); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListAttachedTo(ctx, p1); len(list) != 1 || list[0].ID != p3 || list[0].Status != "Prejudicada" {
		t.Errorf("ListAttachedTo = %+v", list)
	}
	pub, err := s.ListInStates(ctx, domain.PublicStates())
	if err != nil || len(pub) != 1 || pub[0].ID != p3 {
		t.Errorf("ListInStates = %+v (%v)", pub, err)
	}
	if all, _ := s.List(ctx); len(all) != 3 {
		t.Errorf("List = %d", len(all))
	}
	// Apagar a principal solta a apensada (ON DELETE SET NULL).
	if _, err := pool.Exec(ctx, "DELETE FROM proposition.propositions WHERE id = $1", p1); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.FindByID(ctx, p3); got.AttachedToID != nil {
		t.Error("attached_to should be cleared")
	}
	if un, _ := s.ListUnresolvedAttachments(ctx, "PEC 221/2019"); len(un) != 1 {
		t.Errorf("unresolved = %d", len(un))
	}
}

func TestLinksConstraints(t *testing.T) {
	pool, _ := testdb.New(t)
	props := store.NewPropositions(pool)
	links := store.NewLinks(pool)
	ctx := context.Background()
	mkProp(t, props, p1, "pec-221-2019", 221, domain.ChamberCamara)
	mkProp(t, props, p2, "pec-10-2015", 10, domain.ChamberCamara)

	conf, _ := domain.NewConfirmedLink(l1, p1, domain.ChamberCamara, "2192459", "PEC 221/2019", "https://www.camara.leg.br/p", ed, t0)
	if err := links.Create(ctx, conf); err != nil {
		t.Fatal(err)
	}
	sug, _ := domain.NewSuggestedLink(l2, p1, domain.ChamberSenado, "8441243", "PEC 221/2019", "", t0)
	if err := links.Create(ctx, sug); err != nil {
		t.Fatal(err)
	}
	dup, _ := domain.NewSuggestedLink(l3, p1, domain.ChamberSenado, "8441243", "", "", t0)
	if err := links.Create(ctx, dup); !errors.Is(err, domain.ErrLinkExists) {
		t.Errorf("duplicate suggestion: %v", err)
	}
	// A mesma sugestão em outra Proposição é permitida; confirmar nas duas, não.
	other, _ := domain.NewSuggestedLink(l3, p2, domain.ChamberSenado, "8441243", "", "", t0)
	if err := links.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	_ = sug.Confirm(ed, t0)
	if err := links.Confirm(ctx, sug); err != nil {
		t.Fatal(err)
	}
	_ = other.Confirm(ed, t0)
	if err := links.Confirm(ctx, other); !errors.Is(err, domain.ErrLinkTaken) {
		t.Errorf("confirm record taken elsewhere: %v", err)
	}
	if err := links.Confirm(ctx, sug); !errors.Is(err, domain.ErrLinkNotSuggested) {
		t.Errorf("confirm twice: %v", err)
	}
	// Segundo vínculo confirmado na mesma Casa da mesma Proposição.
	second, _ := domain.NewConfirmedLink(l4, p1, domain.ChamberCamara, "999", "", "", ed, t0)
	if err := links.Create(ctx, second); !errors.Is(err, domain.ErrChamberAlreadyLinked) {
		t.Errorf("second confirmed link in chamber: %v", err)
	}

	got, err := links.FindByID(ctx, l2)
	if err != nil || got.State != domain.LinkConfirmed || got.ConfirmedBy != ed || got.ConfirmedAt == nil {
		t.Errorf("confirmed = %+v (%v)", got, err)
	}
	if ids, _ := links.ListConfirmedIDs(ctx); len(ids) != 2 {
		t.Errorf("confirmed ids = %v", ids)
	}
	if byExt, _ := links.ListByExternal(ctx, domain.ChamberSenado, []string{"8441243"}); len(byExt) != 2 {
		t.Errorf("by external = %+v", byExt)
	}
	if err := links.DeleteSuggested(ctx, l2); !errors.Is(err, domain.ErrLinkNotSuggested) {
		t.Errorf("delete confirmed: %v", err)
	}
	if err := links.DeleteSuggested(ctx, l3); err != nil {
		t.Fatal(err)
	}
	got.SyncFailed("fonte fora")
	got.UpdateSource("PEC 221/2019", "https://www25.senado.leg.br/x")
	if err := links.UpdateSync(ctx, got); err != nil {
		t.Fatal(err)
	}
	if got, _ := links.FindByID(ctx, l2); got.LastSyncError != "fonte fora" || got.SourceURL != "https://www25.senado.leg.br/x" {
		t.Errorf("after UpdateSync = %+v", got)
	}
	if bp, _ := links.ListByPropositions(ctx, []string{p1, p2}); len(bp) != 2 {
		t.Errorf("by propositions = %d", len(bp))
	}
	if _, err := links.FindByID(ctx, "x"); !errors.Is(err, domain.ErrLinkNotFound) {
		t.Errorf("invalid id: %v", err)
	}
}
