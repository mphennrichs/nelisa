package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	"github.com/mphennrichs/nelisa/api/internal/proposition/store/dbgen"
)

// --- Versões ---

// Versions implementa app.VersionRepo.
type Versions struct{ pool *pgxpool.Pool }

func NewVersions(pool *pgxpool.Pool) *Versions { return &Versions{pool: pool} }

func (s *Versions) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Versions) Create(ctx context.Context, v domain.Version) error {
	err := s.q(ctx).CreateVersion(ctx, dbgen.CreateVersionParams{
		ID: v.ID, PropositionID: v.PropositionID, Seq: int32(v.Seq), Label: v.Label,
		OfficialTextUrl: v.OfficialTextURL, State: string(v.State), CreatedBy: strPtr(v.CreatedBy), CreatedAt: v.CreatedAt,
	})
	switch uniqueConstraint(err) {
	case "versions_one_pending_key", "versions_seq_key":
		// Duas Versões novas ao mesmo tempo: a segunda perde.
		return domain.ErrPendingVersionExists
	}
	if err != nil {
		return fmt.Errorf("criando versão: %w", err)
	}
	return nil
}

func (s *Versions) FindByID(ctx context.Context, id string) (domain.Version, error) {
	row, err := s.q(ctx).GetVersion(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidText(err) {
		return domain.Version{}, domain.ErrVersionNotFound
	}
	if err != nil {
		return domain.Version{}, fmt.Errorf("lendo versão: %w", err)
	}
	return toVersion(row), nil
}

func (s *Versions) ListByProposition(ctx context.Context, propositionID string) ([]domain.Version, error) {
	rows, err := s.q(ctx).ListVersionsByProposition(ctx, propositionID)
	if isInvalidText(err) {
		return nil, nil
	}
	return versions(rows, err)
}

func (s *Versions) ListByPropositions(ctx context.Context, ids []string) ([]domain.Version, error) {
	return versions(s.q(ctx).ListVersionsByPropositions(ctx, ids))
}

func (s *Versions) UpdateState(ctx context.Context, id string, from, to domain.VersionState) error {
	n, err := s.q(ctx).UpdateVersionState(ctx, dbgen.UpdateVersionStateParams{ID: id, FromState: string(from), State: string(to)})
	if uniqueConstraint(err) != "" {
		return domain.ErrVersionNotPending
	}
	if err != nil {
		return fmt.Errorf("mudando estado da versão: %w", err)
	}
	if n == 0 {
		return domain.ErrVersionNotPending
	}
	return nil
}

func versions(rows []dbgen.PropositionVersion, err error) ([]domain.Version, error) {
	if err != nil {
		return nil, fmt.Errorf("listando versões: %w", err)
	}
	out := make([]domain.Version, 0, len(rows))
	for _, r := range rows {
		out = append(out, toVersion(r))
	}
	return out, nil
}

func toVersion(r dbgen.PropositionVersion) domain.Version {
	v := domain.Version{
		ID: r.ID, PropositionID: r.PropositionID, Seq: int(r.Seq), Label: r.Label,
		OfficialTextURL: r.OfficialTextUrl, State: domain.VersionState(r.State), CreatedAt: r.CreatedAt.UTC(),
	}
	if r.CreatedBy != nil {
		v.CreatedBy = *r.CreatedBy
	}
	return v
}

// --- Conteúdo editorial ---

// Contents implementa app.ContentRepo.
type Contents struct{ pool *pgxpool.Pool }

func NewContents(pool *pgxpool.Pool) *Contents { return &Contents{pool: pool} }

func (s *Contents) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Contents) Create(ctx context.Context, c domain.Content) error {
	err := s.q(ctx).CreateContent(ctx, dbgen.CreateContentParams{
		VersionID: c.VersionID, Summary: c.Summary, ShortSummary: c.ShortSummary, TopicIds: nonNil(c.TopicIDs),
		NeedsConfirmation: c.NeedsConfirmation, Authors: nonNil(c.Authors), Revision: int32(c.Revision),
		UpdatedBy: strPtr(c.UpdatedBy), UpdatedAt: c.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("criando conteúdo: %w", err)
	}
	return nil
}

func (s *Contents) FindByVersion(ctx context.Context, versionID string) (domain.Content, error) {
	row, err := s.q(ctx).GetContent(ctx, versionID)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidText(err) {
		return domain.Content{}, domain.ErrVersionNotFound
	}
	if err != nil {
		return domain.Content{}, fmt.Errorf("lendo conteúdo: %w", err)
	}
	return toContent(row), nil
}

func (s *Contents) ListByVersions(ctx context.Context, ids []string) ([]domain.Content, error) {
	rows, err := s.q(ctx).ListContentsByVersions(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("listando conteúdos: %w", err)
	}
	out := make([]domain.Content, 0, len(rows))
	for _, r := range rows {
		out = append(out, toContent(r))
	}
	return out, nil
}

func (s *Contents) Update(ctx context.Context, c domain.Content, prevRevision int) error {
	n, err := s.q(ctx).UpdateContent(ctx, dbgen.UpdateContentParams{
		VersionID: c.VersionID, Summary: c.Summary, ShortSummary: c.ShortSummary, TopicIds: nonNil(c.TopicIDs),
		NeedsConfirmation: c.NeedsConfirmation, Authors: nonNil(c.Authors), Revision: int32(c.Revision),
		UpdatedBy: strPtr(c.UpdatedBy), UpdatedAt: c.UpdatedAt, PrevRevision: int32(prevRevision),
	})
	if err != nil {
		return fmt.Errorf("gravando conteúdo: %w", err)
	}
	if n == 0 {
		return domain.ErrStaleContent
	}
	return nil
}

func toContent(r dbgen.PropositionVersionContent) domain.Content {
	c := domain.Content{
		VersionID: r.VersionID, Summary: r.Summary, ShortSummary: r.ShortSummary,
		TopicIDs: nonNil(r.TopicIds), NeedsConfirmation: r.NeedsConfirmation, Authors: nonNil(r.Authors),
		Revision: int(r.Revision), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.UpdatedBy != nil {
		c.UpdatedBy = *r.UpdatedBy
	}
	return c
}

// --- Temas ---

// Topics implementa app.TopicRepo.
type Topics struct{ pool *pgxpool.Pool }

func NewTopics(pool *pgxpool.Pool) *Topics { return &Topics{pool: pool} }

func (s *Topics) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Topics) Create(ctx context.Context, t domain.Topic) error {
	err := s.q(ctx).CreateTopic(ctx, dbgen.CreateTopicParams{ID: t.ID, Name: t.Name, Active: t.Active, CreatedAt: t.CreatedAt})
	if uniqueConstraint(err) == "topics_name_key" {
		return domain.ErrTopicNameTaken
	}
	if err != nil {
		return fmt.Errorf("criando tema: %w", err)
	}
	return nil
}

func (s *Topics) FindByID(ctx context.Context, id string) (domain.Topic, error) {
	row, err := s.q(ctx).GetTopic(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidText(err) {
		return domain.Topic{}, domain.ErrTopicNotFound
	}
	if err != nil {
		return domain.Topic{}, fmt.Errorf("lendo tema: %w", err)
	}
	return toTopic(row), nil
}

func (s *Topics) List(ctx context.Context) ([]domain.Topic, error) {
	rows, err := s.q(ctx).ListTopics(ctx)
	if err != nil {
		return nil, fmt.Errorf("listando temas: %w", err)
	}
	out := make([]domain.Topic, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTopic(r))
	}
	return out, nil
}

func (s *Topics) Update(ctx context.Context, t domain.Topic) error {
	err := s.q(ctx).UpdateTopic(ctx, dbgen.UpdateTopicParams{ID: t.ID, Name: t.Name, Active: t.Active})
	if uniqueConstraint(err) == "topics_name_key" {
		return domain.ErrTopicNameTaken
	}
	if err != nil {
		return fmt.Errorf("gravando tema: %w", err)
	}
	return nil
}

func toTopic(r dbgen.PropositionTopic) domain.Topic {
	return domain.Topic{ID: r.ID, Name: r.Name, Active: r.Active, CreatedAt: r.CreatedAt.UTC()}
}
