// Package store é o adaptador Postgres das portas de queue/app. Toda
// consulta usa a transação do contexto quando há uma (db.Conn).
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/queue/app"
	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
	"github.com/mphennrichs/nelisa/api/internal/queue/store/dbgen"
)

const (
	pgInvalidText = "22P02"
	pgForeignKey  = "23503"
)

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// Claims implementa app.ClaimRepo.
type Claims struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Claims { return &Claims{pool: pool} }

func (s *Claims) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Claims) Lock(ctx context.Context, propositionID string) error {
	if err := s.q(ctx).LockClaim(ctx, propositionID); err != nil {
		return fmt.Errorf("travando posse: %w", err)
	}
	return nil
}

func (s *Claims) Find(ctx context.Context, propositionID string) (*domain.Claim, error) {
	row, err := s.q(ctx).GetClaim(ctx, propositionID)
	if errors.Is(err, pgx.ErrNoRows) || pgCode(err) == pgInvalidText {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lendo posse: %w", err)
	}
	c := toClaim(row)
	return &c, nil
}

func (s *Claims) ListByPropositions(ctx context.Context, ids []string) ([]domain.Claim, error) {
	rows, err := s.q(ctx).ListClaims(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("listando posses: %w", err)
	}
	out := make([]domain.Claim, 0, len(rows))
	for _, r := range rows {
		out = append(out, toClaim(r))
	}
	return out, nil
}

func (s *Claims) Save(ctx context.Context, c domain.Claim) error {
	err := s.q(ctx).UpsertClaim(ctx, dbgen.UpsertClaimParams{
		PropositionID: c.PropositionID, UserID: c.UserID, ClaimedAt: c.ClaimedAt, LastActivityAt: c.LastActivityAt,
	})
	if code := pgCode(err); code == pgForeignKey || code == pgInvalidText {
		return app.ErrPropositionNotFound
	}
	if err != nil {
		return fmt.Errorf("gravando posse: %w", err)
	}
	return nil
}

func (s *Claims) Delete(ctx context.Context, propositionID string) error {
	err := s.q(ctx).DeleteClaim(ctx, propositionID)
	if pgCode(err) == pgInvalidText {
		return nil
	}
	if err != nil {
		return fmt.Errorf("soltando posse: %w", err)
	}
	return nil
}

func toClaim(r dbgen.QueueClaim) domain.Claim {
	return domain.Claim{
		PropositionID: r.PropositionID, UserID: r.UserID,
		ClaimedAt: r.ClaimedAt.UTC(), LastActivityAt: r.LastActivityAt.UTC(),
	}
}
