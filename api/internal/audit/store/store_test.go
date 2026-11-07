package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/audit/domain"
	"github.com/mphennrichs/nelisa/api/internal/audit/store"
	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
)

const actor = "6f1c1d1e-0000-4000-8000-000000000001"

func TestInsertAndListByTarget(t *testing.T) {
	pool, _ := testdb.New(t)
	s := store.New(pool)
	ctx := context.Background()
	t0 := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

	e1, _ := domain.NewEvent(actor, "user.create", "user", "u1", t0, map[string]any{"role": "editor"})
	e2, _ := domain.NewEvent("", "user.role_change", "user", "u1", t0.Add(time.Second), nil)
	other, _ := domain.NewEvent(actor, "user.create", "user", "u2", t0, nil)
	for _, e := range []domain.Event{e1, e2, other} {
		if err := s.Insert(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListByTarget(ctx, "user", "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events", len(got))
	}
	if got[0].ActorID != actor || got[0].Metadata["role"] != "editor" || !got[0].At.Equal(t0) {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].ActorID != "" || got[1].Action != "user.role_change" {
		t.Errorf("system event = %+v", got[1])
	}
}

func TestInsertJoinsContextTransaction(t *testing.T) {
	pool, _ := testdb.New(t)
	s := store.New(pool)
	ctx := context.Background()
	e, _ := domain.NewEvent(actor, "user.delete", "user", "u9", time.Now(), nil)

	boom := errors.New("boom")
	err := db.NewTxRunner(pool).WithinTx(ctx, func(ctx context.Context) error {
		if err := s.Insert(ctx, e); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	got, _ := s.ListByTarget(ctx, "user", "u9")
	if len(got) != 0 {
		t.Fatalf("event survived rollback: %+v", got)
	}
}
