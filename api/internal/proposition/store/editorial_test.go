package store_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	"github.com/mphennrichs/nelisa/api/internal/proposition/store"
)

const (
	v1  = "00000000-0000-4000-8000-0000000000b1"
	v2  = "00000000-0000-4000-8000-0000000000b2"
	v3  = "00000000-0000-4000-8000-0000000000b3"
	tp1 = "00000000-0000-4000-8000-0000000000c1"
	tp2 = "00000000-0000-4000-8000-0000000000c2"
	ed2 = "00000000-0000-4000-8000-0000000000e2"
)

func TestVersionsRoundTripAndOnePendingOneCurrent(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	props, vs := store.NewPropositions(pool), store.NewVersions(pool)
	p := mkProp(t, props, p1, "pec-221-2019", 221, domain.ChamberCamara)

	first := domain.NewImportedVersion(v1, p, t0)
	if err := vs.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	got, err := vs.FindByID(ctx, v1)
	if err != nil || got.Seq != 1 || got.Label != "Texto original" || got.CreatedBy != "" || got.State != domain.VersionPending ||
		got.OfficialTextURL != "https://www.camara.leg.br/texto" || !got.CreatedAt.Equal(t0) {
		t.Fatalf("got %+v %v", got, err)
	}
	// Duas em análise: o índice parcial recusa.
	second, _ := domain.NewVersion(v2, p.ID, 2, "Substitutivo", "https://x.leg.br/s", ed, t0)
	if err := vs.Create(ctx, second); !errors.Is(err, domain.ErrPendingVersionExists) {
		t.Fatalf("second pending: %v", err)
	}
	// Aprovação: pending → current, depois outra pending → current com a
	// anterior substituída.
	if err := vs.UpdateState(ctx, v1, domain.VersionPending, domain.VersionCurrent); err != nil {
		t.Fatal(err)
	}
	if err := vs.UpdateState(ctx, v1, domain.VersionPending, domain.VersionCurrent); !errors.Is(err, domain.ErrVersionNotPending) {
		t.Errorf("stale transition: %v", err)
	}
	if err := vs.Create(ctx, second); err != nil {
		t.Fatal(err)
	}
	// Duas vigentes: o índice recusa.
	if err := vs.UpdateState(ctx, v2, domain.VersionPending, domain.VersionCurrent); !errors.Is(err, domain.ErrVersionNotPending) {
		t.Errorf("two current: %v", err)
	}
	if err := db.NewTxRunner(pool).WithinTx(ctx, func(ctx context.Context) error {
		if err := vs.UpdateState(ctx, v1, domain.VersionCurrent, domain.VersionSuperseded); err != nil {
			return err
		}
		return vs.UpdateState(ctx, v2, domain.VersionPending, domain.VersionCurrent)
	}); err != nil {
		t.Fatal(err)
	}
	list, err := vs.ListByProposition(ctx, p.ID)
	if err != nil || len(list) != 2 || list[0].State != domain.VersionSuperseded || list[1].State != domain.VersionCurrent || list[1].CreatedBy != ed {
		t.Fatalf("list = %+v %v", list, err)
	}
	many, err := vs.ListByPropositions(ctx, []string{p.ID, p2})
	if err != nil || len(many) != 2 {
		t.Errorf("many = %+v %v", many, err)
	}
	if _, err := vs.FindByID(ctx, "nao-e-uuid"); !errors.Is(err, domain.ErrVersionNotFound) {
		t.Errorf("bad id: %v", err)
	}
	if l, err := vs.ListByProposition(ctx, "nao-e-uuid"); err != nil || len(l) != 0 {
		t.Errorf("bad proposition id: %v %v", l, err)
	}
}

func TestContentsRoundTripAndOptimisticLock(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	props, vs, cs := store.NewPropositions(pool), store.NewVersions(pool), store.NewContents(pool)
	p := mkProp(t, props, p1, "pec-221-2019", 221, domain.ChamberCamara)
	if err := vs.Create(ctx, domain.NewImportedVersion(v1, p, t0)); err != nil {
		t.Fatal(err)
	}
	empty := domain.NewEmptyContent(v1, t0)
	if err := cs.Create(ctx, empty); err != nil {
		t.Fatal(err)
	}
	got, err := cs.FindByVersion(ctx, v1)
	if err != nil || got.Revision != 0 || got.Authors == nil || got.TopicIDs == nil || got.UpdatedBy != "" {
		t.Fatalf("empty: %+v %v", got, err)
	}
	prev := got.Revision
	got.Apply(domain.ContentEdit{Summary: "Resumo", ShortSummary: "Curto", TopicIDs: []string{tp1, tp2}}, ed, t0)
	if err := cs.Update(ctx, got, prev); err != nil {
		t.Fatal(err)
	}
	// Quem leu a revisão 0 não grava por cima.
	stale := domain.NewEmptyContent(v1, t0)
	stale.Apply(domain.ContentEdit{Summary: "Por cima"}, ed2, t0)
	if err := cs.Update(ctx, stale, 0); !errors.Is(err, domain.ErrStaleContent) {
		t.Fatalf("stale: %v", err)
	}
	after, _ := cs.FindByVersion(ctx, v1)
	if after.Summary != "Resumo" || after.Revision != 1 || !slices.Equal(after.Authors, []string{ed}) ||
		!slices.Equal(after.TopicIDs, []string{tp1, tp2}) || after.UpdatedBy != ed {
		t.Errorf("after = %+v", after)
	}
	list, err := cs.ListByVersions(ctx, []string{v1, v2})
	if err != nil || len(list) != 1 {
		t.Errorf("list = %+v %v", list, err)
	}
	if _, err := cs.FindByVersion(ctx, v3); !errors.Is(err, domain.ErrVersionNotFound) {
		t.Errorf("missing: %v", err)
	}
	// O banco também barra mais de 3 Temas e Resumo curto acima do limite.
	tooMany := after
	tooMany.TopicIDs = []string{tp1, tp2, v1, v2}
	tooMany.Revision = 2
	if err := cs.Update(ctx, tooMany, 1); err == nil {
		t.Error("db accepted 4 topics")
	}
}

func TestTopicsRoundTripAndCaseInsensitiveName(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	ts := store.NewTopics(pool)
	a, _ := domain.NewTopic(tp1, "Saúde", t0)
	b, _ := domain.NewTopic(tp2, "educação", t0)
	for _, tp := range []domain.Topic{a, b} {
		if err := ts.Create(ctx, tp); err != nil {
			t.Fatal(err)
		}
	}
	dup, _ := domain.NewTopic(v1, "SAÚDE", t0)
	if err := ts.Create(ctx, dup); !errors.Is(err, domain.ErrTopicNameTaken) {
		t.Errorf("dup: %v", err)
	}
	b.Name, b.Active = "Saúde", false
	if err := ts.Update(ctx, b); !errors.Is(err, domain.ErrTopicNameTaken) {
		t.Errorf("rename to taken: %v", err)
	}
	b.Name = "Educação"
	if err := ts.Update(ctx, b); err != nil {
		t.Fatal(err)
	}
	list, err := ts.List(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "Educação" || list[0].Active || list[1].Name != "Saúde" {
		t.Errorf("list = %+v %v", list, err)
	}
	if _, err := ts.FindByID(ctx, "x"); !errors.Is(err, domain.ErrTopicNotFound) {
		t.Errorf("bad id: %v", err)
	}
}
