// Package store é o adaptador Postgres da Tramitação. Usa a transação do
// contexto quando há uma (db.Conn).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/progress/domain"
	"github.com/mphennrichs/nelisa/api/internal/progress/store/dbgen"
)

// Store implementa app.EntryRepo.
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Store) Upsert(ctx context.Context, es []domain.Entry, now time.Time) (int, error) {
	inserted := 0
	for _, e := range es {
		isNew, err := s.q(ctx).UpsertEntry(ctx, dbgen.UpsertEntryParams{
			PropositionID: e.PropositionID, Chamber: e.Chamber, ExternalKey: e.ExternalKey, At: e.At,
			Body: e.Body, Description: e.Description, Situation: e.Situation, Dispatch: e.Dispatch, Now: now,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue // evento igual ao guardado
		}
		if err != nil {
			return 0, fmt.Errorf("gravando evento de tramitação: %w", err)
		}
		if isNew {
			inserted++
		}
	}
	return inserted, nil
}

func (s *Store) ListByProposition(ctx context.Context, propositionID string) ([]domain.Entry, error) {
	rows, err := s.q(ctx).ListEntriesByProposition(ctx, propositionID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "22P02" {
		return nil, nil // id que não é UUID: não há eventos
	}
	if err != nil {
		return nil, fmt.Errorf("listando tramitação: %w", err)
	}
	out := make([]domain.Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.Entry{
			PropositionID: r.PropositionID, Chamber: r.Chamber, ExternalKey: r.ExternalKey, At: r.At.UTC(),
			Body: r.Body, Description: r.Description, Situation: r.Situation, Dispatch: r.Dispatch,
		})
	}
	return out, nil
}
