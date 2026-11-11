// Package store é o adaptador Postgres do Radar (ingest/app.RadarRepo).
// Usa a transação do contexto quando há uma (db.Conn).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/store/dbgen"
	"github.com/mphennrichs/nelisa/api/internal/platform/db"
)

// Radar implementa app.RadarRepo.
type Radar struct{ pool *pgxpool.Pool }

func NewRadar(pool *pgxpool.Pool) *Radar { return &Radar{pool: pool} }

func (s *Radar) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Radar) Upsert(ctx context.Context, hits []app.RadarHit, now time.Time) error {
	for _, h := range hits {
		err := s.q(ctx).UpsertRadarItem(ctx, dbgen.UpsertRadarItemParams{
			Chamber: string(h.Chamber), ExternalID: h.ExternalID, Kind: h.Kind, Number: int32(h.Number), Year: int32(h.Year),
			Summary: h.Summary, SourceUrl: h.SourceURL, AgendaAt: h.AgendaAt, PresentedAt: h.PresentedAt, Now: now,
		})
		if err != nil {
			return fmt.Errorf("gravando radar %s/%s: %w", h.Chamber, h.ExternalID, err)
		}
	}
	return nil
}

func (s *Radar) Visible(ctx context.Context, agendaSince, presentedSince time.Time) ([]app.RadarItem, error) {
	rows, err := s.q(ctx).ListVisibleRadarItems(ctx, dbgen.ListVisibleRadarItemsParams{AgendaSince: &agendaSince, PresentedSince: &presentedSince})
	if err != nil {
		return nil, fmt.Errorf("listando radar: %w", err)
	}
	out := make([]app.RadarItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, toItem(r))
	}
	return out, nil
}

func (s *Radar) Dismiss(ctx context.Context, chamber app.ChamberCode, externalID, by string, now time.Time) (app.RadarItem, error) {
	r, err := s.q(ctx).DismissRadarItem(ctx, dbgen.DismissRadarItemParams{Chamber: string(chamber), ExternalID: externalID, Now: &now, By: &by})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.RadarItem{}, app.ErrRadarItemNotFound
	}
	if err != nil {
		return app.RadarItem{}, fmt.Errorf("dispensando item do radar: %w", err)
	}
	return toItem(r), nil
}

func (s *Radar) Purge(ctx context.Context, before time.Time) (int, error) {
	n, err := s.q(ctx).PurgeRadarItems(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("limpando radar: %w", err)
	}
	return int(n), nil
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func toItem(r dbgen.IngestRadarItem) app.RadarItem {
	return app.RadarItem{
		RadarHit: app.RadarHit{
			Chamber: app.ChamberCode(r.Chamber), ExternalID: r.ExternalID, Kind: r.Kind, Number: int(r.Number), Year: int(r.Year),
			Summary: r.Summary, SourceURL: r.SourceUrl, AgendaAt: utc(r.AgendaAt), PresentedAt: utc(r.PresentedAt),
		},
		FirstSeenAt: r.FirstSeenAt.UTC(), DismissedAt: utc(r.DismissedAt),
	}
}

func (s *Radar) LastSeen(ctx context.Context) (*time.Time, error) {
	t, err := s.q(ctx).RadarLastSeen(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lendo última rodada do radar: %w", err)
	}
	return utc(&t), nil
}
