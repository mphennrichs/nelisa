// Package jobs monta o cliente do River (fila de jobs no próprio Postgres)
// para os dois modos que o usam: o worker, que executa jobs, e a API, que
// só enfileira (insert-only). Não conhece nenhum módulo: os workers e os
// jobs periódicos chegam prontos do main.
package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/mphennrichs/nelisa/api/internal/platform/db"
)

// Schema é o schema Postgres das tabelas do River, criado pela migration
// inicial e preenchido pelas migrations do próprio River (platform/migrate).
const Schema = "river"

// DefaultConcurrency é quantos jobs o worker executa ao mesmo tempo. Baixo
// de propósito: os jobs da v1 chamam as APIs oficiais, que os adaptadores
// já limitam por domínio; mais paralelismo aqui só enfileiraria lá.
const DefaultConcurrency = 2

// Client é o cliente do River sobre pgx.
type Client = river.Client[pgx.Tx]

// WorkerOptions configura o cliente do modo worker.
type WorkerOptions struct {
	Workers      *river.Workers
	PeriodicJobs []*river.PeriodicJob
	// Concurrency é o máximo de jobs simultâneos (padrão DefaultConcurrency).
	Concurrency int
	Logger      *slog.Logger
}

// NewWorkerClient monta o cliente que executa jobs. Chame Start para
// começar e Stop para encerrar.
func NewWorkerClient(pool *pgxpool.Pool, opts WorkerOptions) (*Client, error) {
	n := opts.Concurrency
	if n <= 0 {
		n = DefaultConcurrency
	}
	c, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema:       Schema,
		Logger:       opts.Logger,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: n}},
		Workers:      opts.Workers,
		PeriodicJobs: opts.PeriodicJobs,
	})
	if err != nil {
		return nil, fmt.Errorf("montando cliente do river: %w", err)
	}
	return c, nil
}

// NewInsertOnlyClient monta o cliente que só enfileira (modo api): sem
// filas nem workers, não busca nem executa jobs.
func NewInsertOnlyClient(pool *pgxpool.Pool, logger *slog.Logger) (*Client, error) {
	c, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: Schema, Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("montando cliente do river: %w", err)
	}
	return c, nil
}

// Inserter enfileira jobs. Dentro de uma transação aberta por
// db.TxRunner.WithinTx, o job entra nela (InsertTx): só vale se a mudança
// que o motivou for gravada, e some junto se ela desfizer.
type Inserter struct{ client *Client }

func NewInserter(c *Client) Inserter { return Inserter{client: c} }

func (i Inserter) Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error {
	var err error
	if tx, ok := db.TxFrom(ctx); ok {
		_, err = i.client.InsertTx(ctx, tx, args, opts)
	} else {
		_, err = i.client.Insert(ctx, args, opts)
	}
	if err != nil {
		return fmt.Errorf("enfileirando job %s: %w", args.Kind(), err)
	}
	return nil
}

// InsertMany enfileira vários jobs de uma vez, com a mesma regra de
// transação de Insert.
func (i Inserter) InsertMany(ctx context.Context, params []river.InsertManyParams) error {
	if len(params) == 0 {
		return nil
	}
	var err error
	if tx, ok := db.TxFrom(ctx); ok {
		_, err = i.client.InsertManyTx(ctx, tx, params)
	} else {
		_, err = i.client.InsertMany(ctx, params)
	}
	if err != nil {
		return fmt.Errorf("enfileirando %d jobs: %w", len(params), err)
	}
	return nil
}
