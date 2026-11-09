// Package store é o adaptador Postgres das portas de contribution/app.
// Toda consulta usa a transação do contexto quando há uma (db.Conn).
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/contribution/app"
	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/contribution/store/dbgen"
	"github.com/mphennrichs/nelisa/api/internal/platform/db"
)

const (
	pgInvalidText = "22P02"
	pgUnique      = "23505"
)

func pgErr(err error) *pgconn.PgError {
	var e *pgconn.PgError
	if errors.As(err, &e) {
		return e
	}
	return nil
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func val(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Contributions implementa app.ContributionRepo.
type Contributions struct{ pool *pgxpool.Pool }

func NewContributions(pool *pgxpool.Pool) *Contributions { return &Contributions{pool: pool} }

func (s *Contributions) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Contributions) Insert(ctx context.Context, c domain.Contribution) error {
	payload, err := json.Marshal(c.Payload)
	if err != nil {
		return fmt.Errorf("serializando contribuição: %w", err)
	}
	err = s.q(ctx).InsertContribution(ctx, dbgen.InsertContributionParams{
		ID: c.ID, Kind: string(c.Kind), State: string(c.State), TrackingHash: c.TrackingHash,
		PropositionID: ptr(c.PropositionID), Sphere: string(c.Sphere), Payload: payload, SubmittedAt: c.SubmittedAt,
	})
	if e := pgErr(err); e != nil && e.Code == pgUnique && e.ConstraintName == "contributions_tracking_hash_key" {
		return app.ErrTrackingCodeTaken
	}
	if err != nil {
		return fmt.Errorf("gravando contribuição: %w", err)
	}
	return nil
}

func (s *Contributions) FindByTrackingHash(ctx context.Context, hash string) (domain.Contribution, error) {
	row, err := s.q(ctx).GetContributionByTrackingHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Contribution{}, domain.ErrContributionNotFound
	}
	if err != nil {
		return domain.Contribution{}, fmt.Errorf("lendo contribuição: %w", err)
	}
	return toContribution(row)
}

func (s *Contributions) CountPending(ctx context.Context) (int, error) {
	n, err := s.q(ctx).CountPendingContributions(ctx)
	if err != nil {
		return 0, fmt.Errorf("contando contribuições pendentes: %w", err)
	}
	return int(n), nil
}

func toContribution(r dbgen.ContributionContribution) (domain.Contribution, error) {
	var p domain.Payload
	if err := json.Unmarshal(r.Payload, &p); err != nil {
		return domain.Contribution{}, fmt.Errorf("lendo payload da contribuição %s: %w", r.ID, err)
	}
	c := domain.Contribution{
		ID: r.ID, Kind: domain.Kind(r.Kind), State: domain.State(r.State), TrackingHash: r.TrackingHash,
		PropositionID: val(r.PropositionID), Sphere: domain.Sphere(r.Sphere), Payload: p,
		SubmittedAt: r.SubmittedAt.UTC(), DecidedBy: val(r.DecidedBy), PublicReason: r.PublicReason,
		InternalNote: r.InternalNote, CreatedPropositionID: val(r.CreatedPropositionID),
	}
	if r.DecidedAt != nil {
		t := r.DecidedAt.UTC()
		c.DecidedAt = &t
	}
	return c, nil
}

// OfficialSources implementa app.OfficialSourceRepo.
type OfficialSources struct{ pool *pgxpool.Pool }

func NewOfficialSources(pool *pgxpool.Pool) *OfficialSources { return &OfficialSources{pool: pool} }

func (s *OfficialSources) q(ctx context.Context) *dbgen.Queries {
	return dbgen.New(db.Conn(ctx, s.pool))
}

func (s *OfficialSources) List(ctx context.Context) ([]domain.OfficialSource, error) {
	rows, err := s.q(ctx).ListOfficialSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("listando fontes oficiais: %w", err)
	}
	out := make([]domain.OfficialSource, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSource(r))
	}
	return out, nil
}

func (s *OfficialSources) Domains(ctx context.Context) ([]string, error) {
	ds, err := s.q(ctx).ListOfficialDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("listando domínios oficiais: %w", err)
	}
	return ds, nil
}

func (s *OfficialSources) Insert(ctx context.Context, o domain.OfficialSource) error {
	err := s.q(ctx).InsertOfficialSource(ctx, dbgen.InsertOfficialSourceParams{
		ID: o.ID, Domain: o.Domain, Name: o.Name, Sphere: string(o.Sphere), CreatedBy: o.CreatedBy, CreatedAt: o.CreatedAt,
	})
	if e := pgErr(err); e != nil && e.Code == pgUnique {
		return domain.ErrOfficialSourceExists
	}
	if err != nil {
		return fmt.Errorf("gravando fonte oficial: %w", err)
	}
	return nil
}

func (s *OfficialSources) Delete(ctx context.Context, id string) (domain.OfficialSource, error) {
	r, err := s.q(ctx).DeleteOfficialSource(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OfficialSource{}, domain.ErrOfficialSourceNotFound
	}
	if e := pgErr(err); e != nil && e.Code == pgInvalidText {
		return domain.OfficialSource{}, domain.ErrOfficialSourceNotFound
	}
	if err != nil {
		return domain.OfficialSource{}, fmt.Errorf("apagando fonte oficial: %w", err)
	}
	return toSource(r), nil
}

func toSource(r dbgen.ContributionOfficialSource) domain.OfficialSource {
	return domain.OfficialSource{
		ID: r.ID, Domain: r.Domain, Name: r.Name, Sphere: domain.Sphere(r.Sphere),
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(),
	}
}
