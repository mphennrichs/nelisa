// Package db abre o pool do Postgres e oferece a porta de transação usada
// pelos casos de uso.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// txKey guarda no contexto a transação aberta por WithinTx.
type txKey struct{}

// WithinTx roda fn numa transação levada no contexto: todo store que
// obtém a conexão por Conn(ctx, pool) entra nela sem saber disso. É assim
// que o caso de uso de um módulo grava a mudança e o evento de Auditoria
// (outro módulo) juntos, sem que um importe o store do outro. Se o
// contexto já tem transação, fn roda nela (sem aninhar).
func (t TxRunner) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	return t.InTx(ctx, func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

// DBTX é o que os pacotes gerados pelo sqlc pedem; pool e transação servem.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// TxFrom devolve a transação aberta por WithinTx, se houver. Serve a quem
// precisa da transação em si (ex.: o River, que enfileira um job dentro
// dela com InsertTx), não só de algo que execute SQL.
func TxFrom(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// Conn devolve a transação aberta por WithinTx, se houver; senão o pool.
func Conn(ctx context.Context, pool *pgxpool.Pool) DBTX {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return pool
}
