// Package store é o adaptador Postgres das portas de review/app. Toda
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
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
	"github.com/mphennrichs/nelisa/api/internal/review/store/dbgen"
)

const (
	pgUniqueViolation = "23505"
	pgInvalidText     = "22P02"
)

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// Approvals implementa app.ApprovalRepo.
type Approvals struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Approvals { return &Approvals{pool: pool} }

func (s *Approvals) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Approvals) Create(ctx context.Context, a domain.Approval) error {
	err := s.q(ctx).CreateApproval(ctx, dbgen.CreateApprovalParams{
		VersionID: a.VersionID, PropositionID: a.PropositionID, ApprovedBy: a.ApprovedBy, ApprovedAt: a.ApprovedAt,
		FourEyes: a.FourEyes, SecondReviewBy: strPtr(a.SecondReviewBy), SecondReviewAt: a.SecondReviewAt,
	})
	if pgCode(err) == pgUniqueViolation {
		return domain.ErrAlreadyApproved
	}
	if err != nil {
		return fmt.Errorf("gravando aprovação: %w", err)
	}
	return nil
}

func (s *Approvals) FindByVersion(ctx context.Context, versionID string) (domain.Approval, error) {
	row, err := s.q(ctx).GetApproval(ctx, versionID)
	return approvalOrNotFound(row, err)
}

func (s *Approvals) FindByVersionForUpdate(ctx context.Context, versionID string) (domain.Approval, error) {
	row, err := s.q(ctx).GetApprovalForUpdate(ctx, versionID)
	return approvalOrNotFound(row, err)
}

func (s *Approvals) ListByVersions(ctx context.Context, ids []string) ([]domain.Approval, error) {
	return approvals(s.q(ctx).ListApprovalsByVersions(ctx, ids))
}

func (s *Approvals) ListPendingSecondReview(ctx context.Context) ([]domain.Approval, error) {
	return approvals(s.q(ctx).ListPendingSecondReviews(ctx))
}

func (s *Approvals) SetSecondReview(ctx context.Context, a domain.Approval) error {
	n, err := s.q(ctx).SetSecondReview(ctx, dbgen.SetSecondReviewParams{
		VersionID: a.VersionID, SecondReviewBy: strPtr(a.SecondReviewBy), SecondReviewAt: a.SecondReviewAt,
	})
	if err != nil {
		return fmt.Errorf("gravando segunda revisão: %w", err)
	}
	if n == 0 {
		return domain.ErrNoSecondReviewPending
	}
	return nil
}

func approvalOrNotFound(row dbgen.ReviewApproval, err error) (domain.Approval, error) {
	if errors.Is(err, pgx.ErrNoRows) || pgCode(err) == pgInvalidText {
		return domain.Approval{}, domain.ErrApprovalNotFound
	}
	if err != nil {
		return domain.Approval{}, fmt.Errorf("lendo aprovação: %w", err)
	}
	return toApproval(row), nil
}

func approvals(rows []dbgen.ReviewApproval, err error) ([]domain.Approval, error) {
	if err != nil {
		return nil, fmt.Errorf("listando aprovações: %w", err)
	}
	out := make([]domain.Approval, 0, len(rows))
	for _, r := range rows {
		out = append(out, toApproval(r))
	}
	return out, nil
}

func toApproval(r dbgen.ReviewApproval) domain.Approval {
	a := domain.Approval{
		VersionID: r.VersionID, PropositionID: r.PropositionID, ApprovedBy: r.ApprovedBy,
		ApprovedAt: r.ApprovedAt.UTC(), FourEyes: r.FourEyes, SecondReviewAt: utc(r.SecondReviewAt),
	}
	if r.SecondReviewBy != nil {
		a.SecondReviewBy = *r.SecondReviewBy
	}
	return a
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
