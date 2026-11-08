package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
	"github.com/mphennrichs/nelisa/api/internal/review/store/dbgen"
)

// snapshotJSON é o formato do conteúdo guardado em jsonb (before, after,
// proposed). Nomes estáveis: o banco guarda isto para sempre.
type snapshotJSON struct {
	Summary      string   `json:"summary"`
	ShortSummary string   `json:"short_summary"`
	TopicIDs     []string `json:"topic_ids"`
}

func encodeSnapshot(s domain.Snapshot) ([]byte, error) {
	topics := s.TopicIDs
	if topics == nil {
		topics = []string{}
	}
	b, err := json.Marshal(snapshotJSON{Summary: s.Summary, ShortSummary: s.ShortSummary, TopicIDs: topics})
	if err != nil {
		return nil, fmt.Errorf("codificando conteúdo: %w", err)
	}
	return b, nil
}

func decodeSnapshot(b []byte) (domain.Snapshot, error) {
	var s snapshotJSON
	if err := json.Unmarshal(b, &s); err != nil {
		return domain.Snapshot{}, fmt.Errorf("lendo conteúdo guardado: %w", err)
	}
	if s.TopicIDs == nil {
		s.TopicIDs = []string{}
	}
	return domain.Snapshot{Summary: s.Summary, ShortSummary: s.ShortSummary, TopicIDs: s.TopicIDs}, nil
}

// MinorFixes implementa app.MinorFixRepo.
type MinorFixes struct{ pool *pgxpool.Pool }

func NewMinorFixes(pool *pgxpool.Pool) *MinorFixes { return &MinorFixes{pool: pool} }

func (s *MinorFixes) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *MinorFixes) Create(ctx context.Context, f domain.MinorFix) error {
	before, err := encodeSnapshot(f.Before)
	if err != nil {
		return err
	}
	after, err := encodeSnapshot(f.After)
	if err != nil {
		return err
	}
	if err := s.q(ctx).CreateMinorFix(ctx, dbgen.CreateMinorFixParams{
		ID: f.ID, PropositionID: f.PropositionID, VersionID: f.VersionID, Fields: f.Fields,
		Before: before, After: after, CreatedBy: f.CreatedBy, CreatedAt: f.CreatedAt,
	}); err != nil {
		return fmt.Errorf("gravando correção menor: %w", err)
	}
	return nil
}

func (s *MinorFixes) FindForUpdate(ctx context.Context, id string) (domain.MinorFix, error) {
	row, err := s.q(ctx).GetMinorFixForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || pgCode(err) == pgInvalidText {
		return domain.MinorFix{}, domain.ErrMinorFixNotFound
	}
	if err != nil {
		return domain.MinorFix{}, fmt.Errorf("lendo correção menor: %w", err)
	}
	return toMinorFix(row)
}

func (s *MinorFixes) SetChecked(ctx context.Context, f domain.MinorFix) error {
	n, err := s.q(ctx).SetMinorFixChecked(ctx, dbgen.SetMinorFixCheckedParams{
		ID: f.ID, CheckedBy: strPtr(f.CheckedBy), CheckedAt: f.CheckedAt,
	})
	if err != nil {
		return fmt.Errorf("gravando conferência: %w", err)
	}
	if n == 0 {
		return domain.ErrMinorFixChecked
	}
	return nil
}

func (s *MinorFixes) ListUncheckedByProposition(ctx context.Context, propositionID string) ([]domain.MinorFix, error) {
	rows, err := s.q(ctx).ListUncheckedMinorFixesByProposition(ctx, propositionID)
	if pgCode(err) == pgInvalidText {
		return []domain.MinorFix{}, nil
	}
	return minorFixes(rows, err)
}

func (s *MinorFixes) ListUnchecked(ctx context.Context) ([]domain.MinorFix, error) {
	return minorFixes(s.q(ctx).ListUncheckedMinorFixes(ctx))
}

func minorFixes(rows []dbgen.ReviewMinorFix, err error) ([]domain.MinorFix, error) {
	if err != nil {
		return nil, fmt.Errorf("listando correções menores: %w", err)
	}
	out := make([]domain.MinorFix, 0, len(rows))
	for _, r := range rows {
		f, err := toMinorFix(r)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func toMinorFix(r dbgen.ReviewMinorFix) (domain.MinorFix, error) {
	before, err := decodeSnapshot(r.Before)
	if err != nil {
		return domain.MinorFix{}, err
	}
	after, err := decodeSnapshot(r.After)
	if err != nil {
		return domain.MinorFix{}, err
	}
	f := domain.MinorFix{
		ID: r.ID, PropositionID: r.PropositionID, VersionID: r.VersionID, Fields: r.Fields,
		Before: before, After: after, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(), CheckedAt: utc(r.CheckedAt),
	}
	if r.CheckedBy != nil {
		f.CheckedBy = *r.CheckedBy
	}
	return f, nil
}

// Errata implementa app.ErratumRepo.
type Errata struct{ pool *pgxpool.Pool }

func NewErrata(pool *pgxpool.Pool) *Errata { return &Errata{pool: pool} }

func (s *Errata) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Errata) Create(ctx context.Context, e domain.Erratum) error {
	proposed, err := encodeSnapshot(e.Proposed)
	if err != nil {
		return err
	}
	err = s.q(ctx).CreateErratum(ctx, dbgen.CreateErratumParams{
		ID: e.ID, PropositionID: e.PropositionID, VersionID: e.VersionID, Proposed: proposed,
		Said: e.Said, Correct: e.Correct, BaseRevision: int32(e.BaseRevision), CreatedBy: e.CreatedBy, CreatedAt: e.CreatedAt,
	})
	// O índice único de uma pendente por Versão: de duas propostas
	// concorrentes, só uma fica.
	if pgCode(err) == pgUniqueViolation {
		return domain.ErrErratumPending
	}
	if err != nil {
		return fmt.Errorf("gravando errata: %w", err)
	}
	return nil
}

func (s *Errata) FindForUpdate(ctx context.Context, id string) (domain.Erratum, error) {
	row, err := s.q(ctx).GetErratumForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || pgCode(err) == pgInvalidText {
		return domain.Erratum{}, domain.ErrErratumNotFound
	}
	if err != nil {
		return domain.Erratum{}, fmt.Errorf("lendo errata: %w", err)
	}
	return toErratum(row)
}

func (s *Errata) Decide(ctx context.Context, e domain.Erratum) error {
	n, err := s.q(ctx).DecideErratum(ctx, dbgen.DecideErratumParams{
		ID: e.ID, State: string(e.State), DecidedBy: strPtr(e.DecidedBy), DecidedAt: e.DecidedAt, Reason: e.Reason,
	})
	if err != nil {
		return fmt.Errorf("gravando decisão da errata: %w", err)
	}
	if n == 0 {
		return domain.ErrErratumNotPending
	}
	return nil
}

func (s *Errata) HasPending(ctx context.Context, versionID string) (bool, error) {
	n, err := s.q(ctx).CountPendingErrataByVersion(ctx, versionID)
	if err != nil {
		return false, fmt.Errorf("contando erratas pendentes: %w", err)
	}
	return n > 0, nil
}

func (s *Errata) ListByProposition(ctx context.Context, propositionID string) ([]domain.Erratum, error) {
	rows, err := s.q(ctx).ListErrataByProposition(ctx, propositionID)
	if pgCode(err) == pgInvalidText {
		return []domain.Erratum{}, nil
	}
	return errataList(rows, err)
}

func (s *Errata) ListPending(ctx context.Context) ([]domain.Erratum, error) {
	return errataList(s.q(ctx).ListPendingErrata(ctx))
}

func (s *Errata) ListApprovedByVersions(ctx context.Context, ids []string) ([]domain.Erratum, error) {
	return errataList(s.q(ctx).ListApprovedErrataByVersions(ctx, ids))
}

func errataList(rows []dbgen.ReviewErratum, err error) ([]domain.Erratum, error) {
	if err != nil {
		return nil, fmt.Errorf("listando erratas: %w", err)
	}
	out := make([]domain.Erratum, 0, len(rows))
	for _, r := range rows {
		e, err := toErratum(r)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func toErratum(r dbgen.ReviewErratum) (domain.Erratum, error) {
	proposed, err := decodeSnapshot(r.Proposed)
	if err != nil {
		return domain.Erratum{}, err
	}
	e := domain.Erratum{
		ID: r.ID, PropositionID: r.PropositionID, VersionID: r.VersionID, Proposed: proposed,
		Said: r.Said, Correct: r.Correct, BaseRevision: int(r.BaseRevision), State: domain.ErratumState(r.State),
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt.UTC(), DecidedAt: utc(r.DecidedAt), Reason: r.Reason,
	}
	if r.DecidedBy != nil {
		e.DecidedBy = *r.DecidedBy
	}
	return e, nil
}
