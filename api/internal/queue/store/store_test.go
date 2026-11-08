package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
	"github.com/mphennrichs/nelisa/api/internal/queue/app"
	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
	"github.com/mphennrichs/nelisa/api/internal/queue/store"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

const (
	p1  = "00000000-0000-4000-8000-000000000001"
	p2  = "00000000-0000-4000-8000-000000000002"
	ana = "00000000-0000-4000-8000-0000000000e1"
	bia = "00000000-0000-4000-8000-0000000000e2"
)

func TestClaimsRoundTrip(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO proposition.propositions (id, slug, kind, number, year, sphere, origin_chamber, review_state, created_at, updated_at)
		VALUES ($1, 'pec-1-2020', 'PEC', 1, 2020, 'federal', 'camara', 'in_review', $2, $2)`, p1, t0)
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(pool)
	if c, err := s.Find(ctx, p1); err != nil || c != nil {
		t.Fatalf("free: %+v %v", c, err)
	}
	if err := s.Save(ctx, domain.Claim{PropositionID: p1, UserID: ana, ClaimedAt: t0, LastActivityAt: t0}); err != nil {
		t.Fatal(err)
	}
	later := t0.Add(time.Hour)
	if err := s.Save(ctx, domain.Claim{PropositionID: p1, UserID: bia, ClaimedAt: later, LastActivityAt: later}); err != nil {
		t.Fatal(err)
	}
	c, err := s.Find(ctx, p1)
	if err != nil || c == nil || c.UserID != bia || !c.ClaimedAt.Equal(later) {
		t.Fatalf("upsert: %+v %v", c, err)
	}
	list, err := s.ListByPropositions(ctx, []string{p1, p2})
	if err != nil || len(list) != 1 {
		t.Errorf("list: %+v %v", list, err)
	}
	if err := s.Save(ctx, domain.Claim{PropositionID: p2, UserID: ana, ClaimedAt: t0, LastActivityAt: t0}); !errors.Is(err, app.ErrPropositionNotFound) {
		t.Errorf("unknown proposition: %v", err)
	}
	if err := s.Save(ctx, domain.Claim{PropositionID: "nao-uuid", UserID: ana, ClaimedAt: t0, LastActivityAt: t0}); !errors.Is(err, app.ErrPropositionNotFound) {
		t.Errorf("bad id: %v", err)
	}
	if err := s.Delete(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if c, _ := s.Find(ctx, p1); c != nil {
		t.Errorf("deleted: %+v", c)
	}
	if c, err := s.Find(ctx, "nao-uuid"); c != nil || err != nil {
		t.Errorf("bad id find: %+v %v", c, err)
	}
}

// Lock serializa quem mexe na Posse mesmo sem linha: duas transações
// concorrentes não assumem a mesma Proposição ao mesmo tempo.
func TestLockSerializesClaims(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO proposition.propositions (id, slug, kind, number, year, sphere, origin_chamber, review_state, created_at, updated_at)
		VALUES ($1, 'pec-1-2020', 'PEC', 1, 2020, 'federal', 'camara', 'in_review', $2, $2)`, p1, t0)
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(pool)
	tx := db.NewTxRunner(pool)
	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	for _, user := range []string{ana, bia} {
		wg.Add(1)
		go func(user string) {
			defer wg.Done()
			err := tx.WithinTx(ctx, func(ctx context.Context) error {
				if err := s.Lock(ctx, p1); err != nil {
					return err
				}
				c, err := s.Find(ctx, p1)
				if err != nil {
					return err
				}
				if c != nil {
					return domain.ErrClaimTaken
				}
				time.Sleep(50 * time.Millisecond)
				return s.Save(ctx, domain.Claim{PropositionID: p1, UserID: user, ClaimedAt: t0, LastActivityAt: t0})
			})
			if err == nil {
				mu.Lock()
				winners++
				mu.Unlock()
			} else if !errors.Is(err, domain.ErrClaimTaken) {
				t.Error(err)
			}
		}(user)
	}
	wg.Wait()
	if winners != 1 {
		t.Errorf("winners = %d", winners)
	}
}
