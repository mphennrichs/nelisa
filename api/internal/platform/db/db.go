// Package db abre o pool do Postgres e oferece a porta de transação usada
// pelos casos de uso.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Open abre o pool e confere a conexão.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("abrindo pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectando ao postgres: %w", err)
	}
	return pool, nil
}

// TxRunner roda fn dentro de uma transação: commit se fn devolver nil,
// rollback caso contrário. Os casos de uso dependem desta porta para gravar
// a mudança e o evento de Auditoria juntos.
type TxRunner struct {
	pool *pgxpool.Pool
}

func NewTxRunner(pool *pgxpool.Pool) TxRunner { return TxRunner{pool: pool} }

func (t TxRunner) InTx(ctx context.Context, fn func(tx pgx.Tx) error) (err error) {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrindo transação: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, rbErr)
			}
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
