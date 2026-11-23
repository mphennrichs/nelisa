package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/contribution/store/dbgen"
	"github.com/mphennrichs/nelisa/api/internal/platform/db"
)

// RateCounter implementa app.RateCounter.
type RateCounter struct{ pool *pgxpool.Pool }

func NewRateCounter(pool *pgxpool.Pool) *RateCounter { return &RateCounter{pool: pool} }

func (s *RateCounter) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *RateCounter) DailyKey(ctx context.Context, day time.Time, candidate []byte) ([]byte, error) {
	key, err := s.q(ctx).EnsureRateKey(ctx, dbgen.EnsureRateKeyParams{Day: day, Key: candidate})
	if err != nil {
		return nil, fmt.Errorf("lendo chave do dia: %w", err)
	}
	return key, nil
}

func (s *RateCounter) Hit(ctx context.Context, subject []byte, bucket domain.RateBucket, window time.Time) (int, error) {
	n, err := s.q(ctx).HitRate(ctx, dbgen.HitRateParams{Subject: subject, Bucket: string(bucket), WindowStart: window})
	if err != nil {
		return 0, fmt.Errorf("contando tentativa: %w", err)
	}
	return int(n), nil
}

func (s *RateCounter) Purge(ctx context.Context, keysBefore, hitsBefore time.Time) error {
	q := s.q(ctx)
	if _, err := q.PurgeRateKeys(ctx, keysBefore); err != nil {
		return fmt.Errorf("apagando chaves do dia: %w", err)
	}
	if _, err := q.PurgeRateHits(ctx, hitsBefore); err != nil {
		return fmt.Errorf("apagando contadores: %w", err)
	}
	return nil
}
