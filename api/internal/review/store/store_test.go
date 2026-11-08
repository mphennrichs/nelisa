package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/platform/testdb"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
	"github.com/mphennrichs/nelisa/api/internal/review/store"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

const (
	p1  = "00000000-0000-4000-8000-000000000001"
	v1  = "00000000-0000-4000-8000-0000000000b1"
	v2  = "00000000-0000-4000-8000-0000000000b2"
	adm = "00000000-0000-4000-8000-0000000000e1"
	rev = "00000000-0000-4000-8000-0000000000e2"
)

// seed cria a Proposição e duas Versões direto no banco: o review só
// guarda aprovações que apontam para elas.
func seed(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO proposition.propositions (id, slug, kind, number, year, sphere, origin_chamber, review_state, created_at, updated_at)
		VALUES ($1, 'pec-1-2020', 'PEC', 1, 2020, 'federal', 'camara', 'in_review', $2, $2)`, p1, t0)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range []string{v1, v2} {
		_, err := pool.Exec(ctx, `INSERT INTO proposition.versions (id, proposition_id, seq, label, state, created_at)
			VALUES ($1, $2, $3, 'V', 'superseded', $4)`, v, p1, i+1, t0)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestApprovalsRoundTripAndSecondReview(t *testing.T) {
	pool, _ := testdb.New(t)
	seed(t, pool)
	ctx := context.Background()
	s := store.New(pool)

	four := domain.Approval{VersionID: v1, PropositionID: p1, ApprovedBy: rev, ApprovedAt: t0, FourEyes: true}
	self := domain.Approval{VersionID: v2, PropositionID: p1, ApprovedBy: adm, ApprovedAt: t0.Add(time.Hour)}
	for _, a := range []domain.Approval{four, self} {
		if err := s.Create(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Create(ctx, four); !errors.Is(err, domain.ErrAlreadyApproved) {
		t.Errorf("duplicate: %v", err)
	}
	got, err := s.FindByVersion(ctx, v2)
	if err != nil || got.FourEyes || got.ApprovedBy != adm || !got.ApprovedAt.Equal(t0.Add(time.Hour)) || got.SecondReviewAt != nil {
		t.Fatalf("got %+v %v", got, err)
	}
	pending, err := s.ListPendingSecondReview(ctx)
	if err != nil || len(pending) != 1 || pending[0].VersionID != v2 {
		t.Fatalf("pending = %+v %v", pending, err)
	}
	// O banco recusa que o aprovador faça a segunda revisão.
	at := t0.Add(2 * time.Hour)
	bad := got
	bad.SecondReviewBy, bad.SecondReviewAt = adm, &at
	if err := s.SetSecondReview(ctx, bad); err == nil {
		t.Error("db accepted approver as second reviewer")
	}
	got.SecondReviewBy, got.SecondReviewAt = rev, &at
	if err := s.SetSecondReview(ctx, got); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSecondReview(ctx, got); !errors.Is(err, domain.ErrNoSecondReviewPending) {
		t.Errorf("twice: %v", err)
	}
	four.SecondReviewBy, four.SecondReviewAt = adm, &at
	if err := s.SetSecondReview(ctx, four); !errors.Is(err, domain.ErrNoSecondReviewPending) {
		t.Errorf("on four eyes: %v", err)
	}
	list, err := s.ListByVersions(ctx, []string{v1, v2})
	if err != nil || len(list) != 2 || list[0].Badge() != 2 || list[1].Badge() != 2 {
		t.Errorf("list = %+v %v", list, err)
	}
	if p, _ := s.ListPendingSecondReview(ctx); len(p) != 0 {
		t.Errorf("pending after review = %+v", p)
	}
	if _, err := s.FindByVersionForUpdate(ctx, "nao-uuid"); !errors.Is(err, domain.ErrApprovalNotFound) {
		t.Errorf("bad id: %v", err)
	}
}
