package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
)

func TestWithinTxCommitsAndRollsBackEverythingInContext(t *testing.T) {
	pool, _ := testdb.New(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "CREATE TABLE t (v int)"); err != nil {
		t.Fatal(err)
	}
	tx := db.NewTxRunner(pool)
	insert := func(ctx context.Context, v int) error {
		_, err := db.Conn(ctx, pool).Exec(ctx, "INSERT INTO t VALUES ($1)", v)
		return err
	}

	boom := errors.New("boom")
	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := insert(ctx, 1); err != nil {
			return err
		}
		// a nested call joins the same transaction
		return tx.WithinTx(ctx, func(ctx context.Context) error {
			if err := insert(ctx, 2); err != nil {
				return err
			}
			return boom
		})
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := count(t, pool); n != 0 {
		t.Fatalf("rollback left %d rows", n)
	}

	if err := tx.WithinTx(ctx, func(ctx context.Context) error { return insert(ctx, 3) }); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool); n != 1 {
		t.Fatalf("commit left %d rows, want 1", n)
	}
}

func count(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM t").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
