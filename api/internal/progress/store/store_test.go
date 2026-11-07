package store_test

import (
	"context"
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
