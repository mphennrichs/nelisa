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

// Comments implementa app.CommentRepo.
type Comments struct{ pool *pgxpool.Pool }

func NewComments(pool *pgxpool.Pool) *Comments { return &Comments{pool: pool} }

func (s *Comments) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Comments) Create(ctx context.Context, c domain.Comment) error {
	err := s.q(ctx).CreateComment(ctx, dbgen.CreateCommentParams{
		ID: c.ID, VersionID: c.VersionID, ProvisionAddress: c.ProvisionAddress, Body: c.Body,
		NeedsConfirmation: c.NeedsConfirmation, CopiedFrom: strPtr(c.CopiedFrom), Authors: nonNil(c.Authors),
		Revision: int32(c.Revision), CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt,
		UpdatedBy: strPtr(c.UpdatedBy), UpdatedAt: c.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("criando comentário: %w", err)
	}
	return nil
}

func (s *Comments) FindForUpdate(ctx context.Context, id string) (domain.Comment, error) {
	row, err := s.q(ctx).GetCommentForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidText(err) {
		return domain.Comment{}, domain.ErrCommentNotFound
	}
	if err != nil {
		return domain.Comment{}, fmt.Errorf("lendo comentário: %w", err)
	}
	return toComment(row), nil
}

func (s *Comments) ListByVersions(ctx context.Context, versionIDs []string) ([]domain.Comment, error) {
	if len(versionIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListComments(ctx, versionIDs)
	if err != nil {
		return nil, fmt.Errorf("listando comentários: %w", err)
	}
	out := make([]domain.Comment, len(rows))
	for i, r := range rows {
		out[i] = toComment(r)
	}
	return out, nil
}

func (s *Comments) Update(ctx context.Context, c domain.Comment, prevRevision int) error {
	n, err := s.q(ctx).UpdateComment(ctx, dbgen.UpdateCommentParams{
		Body: c.Body, NeedsConfirmation: c.NeedsConfirmation, Authors: nonNil(c.Authors), Revision: int32(c.Revision),
		UpdatedBy: strPtr(c.UpdatedBy), UpdatedAt: c.UpdatedAt, DeletedAt: c.DeletedAt,
		ID: c.ID, PrevRevision: int32(prevRevision),
	})
	if err != nil {
		return fmt.Errorf("gravando comentário: %w", err)
	}
	if n == 0 {
		return domain.ErrStaleContent
	}
	return nil
}

func toComment(r dbgen.PropositionComment) domain.Comment {
	c := domain.Comment{
		ID: r.ID, VersionID: r.VersionID, ProvisionAddress: r.ProvisionAddress, Body: r.Body,
		NeedsConfirmation: r.NeedsConfirmation, Authors: nonNil(r.Authors), Revision: int(r.Revision),
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
	if r.CopiedFrom != nil {
		c.CopiedFrom = *r.CopiedFrom
	}
	if r.UpdatedBy != nil {
		c.UpdatedBy = *r.UpdatedBy
	}
	if r.DeletedAt != nil {
		t := r.DeletedAt.UTC()
		c.DeletedAt = &t
	}
	return c
}
