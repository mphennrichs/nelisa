package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
	"github.com/mphennrichs/nelisa/api/internal/progress/domain"
	"github.com/mphennrichs/nelisa/api/internal/progress/store"
)

var t0 = time.Date(2019, 11, 26, 12, 0, 0, 0, time.UTC)

const prop = "00000000-0000-4000-8000-000000000001"

func entry(t *testing.T, chamber, key string, at time.Time, desc string) domain.Entry {
	t.Helper()
	e, err := domain.NewEntry(prop, chamber, key, at, "PLEN", desc, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestUpsertIsIdempotentAndTimelineOrdered(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO proposition.propositions
		(id, slug, kind, number, year, sphere, origin_chamber, review_state, created_at, updated_at)
		VALUES ($1, 'pec-221-2019', 'PEC', 221, 2019, 'federal', 'camara', 'draft', now(), now())`, prop); err != nil {
		t.Fatal(err)
	}
	s := store.New(pool)
	batch := []domain.Entry{
		entry(t, "camara", "2-200", t0.Add(48*time.Hour), "Aprovada"),
		entry(t, "camara", "1-100", t0, "Apresentação"),
		entry(t, "senado", "77", t0, "Recebida"), // mesmo instante: Casa desempata
	}
	n, err := s.Upsert(ctx, batch, t0)
	if err != nil || n != 3 {
		t.Fatalf("first upsert: %d %v", n, err)
	}
	n, err = s.Upsert(ctx, batch, t0.Add(time.Hour))
	if err != nil || n != 0 {
		t.Fatalf("second upsert must insert nothing: %d %v", n, err)
	}
	corrected := entry(t, "camara", "2-200", t0.Add(48*time.Hour), "Aprovada em 2º turno")
	if n, err := s.Upsert(ctx, []domain.Entry{corrected}, t0.Add(2*time.Hour)); err != nil || n != 0 {
		t.Fatalf("correction: %d %v", n, err)
	}
	tl, err := s.ListByProposition(ctx, prop)
	if err != nil || len(tl) != 3 {
		t.Fatalf("timeline: %v %+v", err, tl)
	}
	got := []string{tl[0].Chamber + tl[0].ExternalKey, tl[1].Chamber + tl[1].ExternalKey, tl[2].Chamber + tl[2].ExternalKey}
	if got[0] != "camara1-100" || got[1] != "senado77" || got[2] != "camara2-200" || tl[2].Description != "Aprovada em 2º turno" {
		t.Errorf("timeline = %v / %+v", got, tl[2])
	}
	if es, err := s.ListByProposition(ctx, "nao-e-uuid"); err != nil || len(es) != 0 {
		t.Errorf("invalid id: %v %v", es, err)
	}
}

func TestManualEntries(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO proposition.propositions
		(id, slug, kind, number, year, sphere, origin_chamber, review_state, created_at, updated_at)
		VALUES ($1, 'pec-221-2019', 'PEC', 221, 2019, 'federal', 'camara', 'draft', now(), now())`, prop); err != nil {
		t.Fatal(err)
	}
	s := store.New(pool)
	m, err := domain.NewManualEntry(prop, "camara", "c1", t0, "Votação adiada", "https://g1.globo.com/x")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertManual(ctx, m, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertManual(ctx, m, t0); !errors.Is(err, domain.ErrManualEntryExists) {
		t.Errorf("de novo: %v", err)
	}
	// A Sincronização convive com o evento manual e não o toca.
	if _, err := s.Upsert(ctx, []domain.Entry{entry(t, "camara", "1-100", t0, "Adiada")}, t0); err != nil {
		t.Fatal(err)
	}
	tl, err := s.ListByProposition(ctx, prop)
	if err != nil || len(tl) != 2 {
		t.Fatalf("timeline: %v %+v", err, tl)
	}
	var manual domain.Entry
	for _, e := range tl {
		if e.Manual {
			manual = e
		}
	}
	if manual.SourceURL != "https://g1.globo.com/x" || manual.ContributionID() != "c1" {
		t.Errorf("manual = %+v", manual)
	}
	// O banco recusa evento manual sem fonte e evento da fonte com chave manual.
	if _, err := pool.Exec(ctx, `INSERT INTO progress.entries (proposition_id, chamber, external_key, at, manual, created_at, updated_at)
		VALUES ($1, 'camara', 'contribution:c9', now(), true, now(), now())`, prop); err == nil {
		t.Error("manual sem fonte deveria falhar")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO progress.entries (proposition_id, chamber, external_key, at, created_at, updated_at)
		VALUES ($1, 'camara', 'contribution:c9', now(), now(), now())`, prop); err == nil {
		t.Error("chave manual sem manual deveria falhar")
	}
	if ok, err := s.DeleteManual(ctx, domain.ManualKey("c1")); err != nil || !ok {
		t.Errorf("delete = %v, %v", ok, err)
	}
	// Só apaga evento manual.
	if ok, err := s.DeleteManual(ctx, "1-100"); err != nil || ok {
		t.Errorf("delete da fonte = %v, %v", ok, err)
	}
}
