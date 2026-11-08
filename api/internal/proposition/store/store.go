// Package store é o adaptador Postgres das portas de proposition/app. Toda
// consulta usa a transação do contexto quando há uma (db.Conn).
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
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	"github.com/mphennrichs/nelisa/api/internal/proposition/store/dbgen"
)

const (
	pgUniqueViolation = "23505"
	// pgInvalidText: um id que não é UUID. Para quem pergunta, é "não existe".
	pgInvalidText = "22P02"
)

func isInvalidText(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgInvalidText
}

func uniqueConstraint(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return pgErr.ConstraintName
	}
	return ""
}

// --- Proposições ---

// Propositions implementa app.PropositionRepo.
type Propositions struct{ pool *pgxpool.Pool }

func NewPropositions(pool *pgxpool.Pool) *Propositions { return &Propositions{pool: pool} }

func (s *Propositions) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Propositions) Create(ctx context.Context, p domain.Proposition) error {
	err := s.q(ctx).CreateProposition(ctx, dbgen.CreatePropositionParams{
		ID: p.ID, Slug: p.Slug, Kind: p.Kind, Number: int32(p.Number), Year: int32(p.Year),
		Sphere: string(p.Sphere), OriginChamber: string(p.OriginChamber),
		OfficialSummary: p.OfficialSummary, PresentedAt: p.PresentedAt, OfficialTextUrl: p.OfficialTextURL,
		Authors: nonNil(p.Authors), Status: p.Status, StatusAt: p.StatusAt,
		AttachedTo: p.AttachedToID, AttachedToName: p.AttachedToName, Superseded: p.Superseded,
		ReviewState: string(p.ReviewState), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	})
	switch uniqueConstraint(err) {
	case "propositions_identity_key":
		return domain.ErrPropositionExists
	case "propositions_slug_key":
		return domain.ErrSlugTaken
	}
	if err != nil {
		return fmt.Errorf("criando proposição: %w", err)
	}
	return nil
}

func (s *Propositions) FindByID(ctx context.Context, id string) (domain.Proposition, error) {
	row, err := s.q(ctx).GetPropositionByID(ctx, id)
	return propositionOrNotFound(row, err)
}

func (s *Propositions) FindByIDForUpdate(ctx context.Context, id string) (domain.Proposition, error) {
	row, err := s.q(ctx).GetPropositionByIDForUpdate(ctx, id)
	return propositionOrNotFound(row, err)
}

func (s *Propositions) FindBySlug(ctx context.Context, slug string) (domain.Proposition, error) {
	row, err := s.q(ctx).GetPropositionBySlug(ctx, slug)
	return propositionOrNotFound(row, err)
}

func (s *Propositions) SlugExists(ctx context.Context, slug string) (bool, error) {
	ok, err := s.q(ctx).SlugExists(ctx, slug)
	if err != nil {
		return false, fmt.Errorf("conferindo slug: %w", err)
	}
	return ok, nil
}

func (s *Propositions) FindByName(ctx context.Context, kind string, number, year int) ([]domain.Proposition, error) {
	rows, err := s.q(ctx).FindPropositionsByName(ctx, dbgen.FindPropositionsByNameParams{Kind: kind, Number: int32(number), Year: int32(year)})
	return propositions(rows, err)
}

func (s *Propositions) List(ctx context.Context) ([]domain.Proposition, error) {
	return propositions(s.q(ctx).ListPropositions(ctx))
}

func (s *Propositions) ListInStates(ctx context.Context, states []domain.ReviewState) ([]domain.Proposition, error) {
	raw := make([]string, 0, len(states))
	for _, st := range states {
		raw = append(raw, string(st))
	}
	return propositions(s.q(ctx).ListPropositionsInStates(ctx, raw))
}

func (s *Propositions) ListAttachedTo(ctx context.Context, principalID string) ([]domain.Proposition, error) {
	return propositions(s.q(ctx).ListAttachedTo(ctx, &principalID))
}

func (s *Propositions) ListUnresolvedAttachments(ctx context.Context, name string) ([]domain.Proposition, error) {
	return propositions(s.q(ctx).ListUnresolvedAttachments(ctx, name))
}

func (s *Propositions) Update(ctx context.Context, p domain.Proposition) error {
	err := s.q(ctx).UpdateProposition(ctx, dbgen.UpdatePropositionParams{
		ID: p.ID, OfficialSummary: p.OfficialSummary, PresentedAt: p.PresentedAt, OfficialTextUrl: p.OfficialTextURL,
		Authors: nonNil(p.Authors), Status: p.Status, StatusAt: p.StatusAt,
		AttachedTo: p.AttachedToID, AttachedToName: p.AttachedToName, Superseded: p.Superseded,
		ReviewState: string(p.ReviewState), UpdatedAt: p.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("atualizando proposição: %w", err)
	}
	return nil
}

func propositionOrNotFound(row dbgen.PropositionProposition, err error) (domain.Proposition, error) {
	if errors.Is(err, pgx.ErrNoRows) || isInvalidText(err) {
		return domain.Proposition{}, domain.ErrPropositionNotFound
	}
	if err != nil {
		return domain.Proposition{}, fmt.Errorf("lendo proposição: %w", err)
	}
	return toProposition(row), nil
}

func propositions(rows []dbgen.PropositionProposition, err error) ([]domain.Proposition, error) {
	if err != nil {
		return nil, fmt.Errorf("listando proposições: %w", err)
	}
	out := make([]domain.Proposition, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProposition(r))
	}
	return out, nil
}

func toProposition(r dbgen.PropositionProposition) domain.Proposition {
	return domain.Proposition{
		ID: r.ID, Slug: r.Slug, Kind: r.Kind, Number: int(r.Number), Year: int(r.Year),
		Sphere: domain.Sphere(r.Sphere), OriginChamber: domain.Chamber(r.OriginChamber),
		OfficialData: domain.OfficialData{
			OfficialSummary: r.OfficialSummary, PresentedAt: utc(r.PresentedAt), OfficialTextURL: r.OfficialTextUrl,
			Authors: r.Authors, Status: r.Status, AttachedToName: r.AttachedToName, Superseded: r.Superseded,
		},
		StatusAt: utc(r.StatusAt), AttachedToID: r.AttachedTo, ReviewState: domain.ReviewState(r.ReviewState),
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

// --- Vínculos de fonte ---

// Links implementa app.LinkRepo.
type Links struct{ pool *pgxpool.Pool }

func NewLinks(pool *pgxpool.Pool) *Links { return &Links{pool: pool} }

func (s *Links) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

// linkConstraintErr traduz os índices únicos de source_links.
func linkConstraintErr(err error) error {
	switch uniqueConstraint(err) {
	case "source_links_confirmed_external_key":
		return domain.ErrLinkTaken
	case "source_links_confirmed_chamber_key":
		return domain.ErrChamberAlreadyLinked
	case "source_links_proposition_external_key":
		return domain.ErrLinkExists
	}
	return nil
}

func (s *Links) Create(ctx context.Context, l domain.SourceLink) error {
	err := s.q(ctx).CreateLink(ctx, dbgen.CreateLinkParams{
		ID: l.ID, PropositionID: l.PropositionID, Chamber: string(l.Chamber), ExternalID: l.ExternalID,
		State: string(l.State), SourceName: l.SourceName, SourceUrl: l.SourceURL,
		ConfirmedBy: strPtr(l.ConfirmedBy), ConfirmedAt: l.ConfirmedAt,
		LastSyncedAt: l.LastSyncedAt, LastSyncError: l.LastSyncError, CreatedAt: l.CreatedAt,
	})
	if e := linkConstraintErr(err); e != nil {
		return e
	}
	if err != nil {
		return fmt.Errorf("criando vínculo: %w", err)
	}
	return nil
}

func (s *Links) FindByID(ctx context.Context, id string) (domain.SourceLink, error) {
	row, err := s.q(ctx).GetLink(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidText(err) {
		return domain.SourceLink{}, domain.ErrLinkNotFound
	}
	if err != nil {
		return domain.SourceLink{}, fmt.Errorf("lendo vínculo: %w", err)
	}
	return toLink(row), nil
}

func (s *Links) ListByProposition(ctx context.Context, propositionID string) ([]domain.SourceLink, error) {
	rows, err := s.q(ctx).ListLinksByProposition(ctx, propositionID)
	if isInvalidText(err) {
		return nil, nil
	}
	return links(rows, err)
}

func (s *Links) ListByPropositions(ctx context.Context, ids []string) ([]domain.SourceLink, error) {
	return links(s.q(ctx).ListLinksByPropositions(ctx, ids))
}

func (s *Links) ListByExternal(ctx context.Context, chamber domain.Chamber, externalIDs []string) ([]domain.SourceLink, error) {
	return links(s.q(ctx).ListLinksByExternal(ctx, dbgen.ListLinksByExternalParams{Chamber: string(chamber), ExternalIds: externalIDs}))
}

func (s *Links) ListConfirmedIDs(ctx context.Context) ([]string, error) {
	ids, err := s.q(ctx).ListConfirmedLinkIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listando vínculos confirmados: %w", err)
	}
	return ids, nil
}

func (s *Links) Confirm(ctx context.Context, l domain.SourceLink) error {
	n, err := s.q(ctx).ConfirmLink(ctx, dbgen.ConfirmLinkParams{ID: l.ID, ConfirmedBy: strPtr(l.ConfirmedBy), ConfirmedAt: l.ConfirmedAt})
	if e := linkConstraintErr(err); e != nil {
		return e
	}
	if err != nil {
		return fmt.Errorf("confirmando vínculo: %w", err)
	}
	if n == 0 {
		return domain.ErrLinkNotSuggested
	}
	return nil
}

func (s *Links) DeleteSuggested(ctx context.Context, id string) error {
	n, err := s.q(ctx).DeleteSuggestedLink(ctx, id)
	if err != nil {
		return fmt.Errorf("descartando vínculo: %w", err)
	}
	if n == 0 {
		return domain.ErrLinkNotSuggested
	}
	return nil
}

func (s *Links) UpdateSync(ctx context.Context, l domain.SourceLink) error {
	err := s.q(ctx).UpdateLinkSync(ctx, dbgen.UpdateLinkSyncParams{
		ID: l.ID, LastSyncedAt: l.LastSyncedAt, LastSyncError: l.LastSyncError,
		SourceName: l.SourceName, SourceUrl: l.SourceURL,
	})
	if err != nil {
		return fmt.Errorf("gravando sincronização do vínculo: %w", err)
	}
	return nil
}

func links(rows []dbgen.PropositionSourceLink, err error) ([]domain.SourceLink, error) {
	if err != nil {
		return nil, fmt.Errorf("listando vínculos: %w", err)
	}
	out := make([]domain.SourceLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, toLink(r))
	}
	return out, nil
}

func toLink(r dbgen.PropositionSourceLink) domain.SourceLink {
	l := domain.SourceLink{
		ID: r.ID, PropositionID: r.PropositionID, Chamber: domain.Chamber(r.Chamber), ExternalID: r.ExternalID,
		State: domain.LinkState(r.State), SourceName: r.SourceName, SourceURL: r.SourceUrl,
		ConfirmedAt: utc(r.ConfirmedAt), LastSyncedAt: utc(r.LastSyncedAt), LastSyncError: r.LastSyncError,
		CreatedAt: r.CreatedAt.UTC(),
	}
	if r.ConfirmedBy != nil {
		l.ConfirmedBy = *r.ConfirmedBy
	}
	return l
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nonNil: a coluna authors é NOT NULL; lista vazia vira '{}'.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
