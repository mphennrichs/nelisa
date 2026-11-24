package store_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/contribution/store"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
)

func TestRateCounter(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	s := store.NewRateCounter(pool)
	day := domain.RateDay(t0)
	window := domain.RateWindow(t0)

	first := bytes.Repeat([]byte{1}, 32)
	k, err := s.DailyKey(ctx, day, first)
	if err != nil || !bytes.Equal(k, first) {
		t.Fatalf("primeira chave = %x, %v", k, err)
	}
	// Outra instância sorteou outra chave no mesmo dia: vale a gravada.
	k, err = s.DailyKey(ctx, day, bytes.Repeat([]byte{2}, 32))
	if err != nil || !bytes.Equal(k, first) {
		t.Fatalf("segunda chave = %x, %v; want a primeira", k, err)
	}

	subj := domain.RateSubject(first, "203.0.113.7")
	for want := 1; want <= 3; want++ {
		n, err := s.Hit(ctx, subj, domain.BucketSubmit, window)
		if err != nil || n != want {
			t.Fatalf("hit = %d, %v; want %d", n, err, want)
		}
	}
	// Balde e janela separam os contadores.
	if n, _ := s.Hit(ctx, subj, domain.BucketLookup, window); n != 1 {
		t.Errorf("outro balde = %d", n)
	}
	if n, _ := s.Hit(ctx, subj, domain.BucketSubmit, window.Add(-time.Hour)); n != 1 {
		t.Errorf("hora anterior = %d", n)
	}
	if _, err := s.DailyKey(ctx, day.Add(-24*time.Hour), bytes.Repeat([]byte{3}, 32)); err != nil {
		t.Fatal(err)
	}

	if err := s.Purge(ctx, day, window); err != nil {
		t.Fatal(err)
	}
	var keys, hits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM contribution.rate_keys`).Scan(&keys); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM contribution.rate_hits`).Scan(&hits); err != nil {
		t.Fatal(err)
	}
	if keys != 1 || hits != 2 {
		t.Errorf("depois do purge: %d chaves, %d contadores; want 1 e 2", keys, hits)
	}
}
