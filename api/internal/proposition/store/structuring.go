package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain/structuring"
	"github.com/mphennrichs/nelisa/api/internal/proposition/store/dbgen"
)

// Structurings implementa app.StructuringRepo.
type Structurings struct{ pool *pgxpool.Pool }

func NewStructurings(pool *pgxpool.Pool) *Structurings { return &Structurings{pool: pool} }

func (s *Structurings) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

type warningJSON struct {
	Code    string `json:"code"`
	Line    int    `json:"line"`
	Address string `json:"address,omitempty"`
}

func encodeWarnings(ws []structuring.Warning) ([]byte, error) {
	out := make([]warningJSON, len(ws))
	for i, w := range ws {
		out[i] = warningJSON(w)
	}
	return json.Marshal(out)
}

func (s *Structurings) Create(ctx context.Context, st domain.Structuring) error {
	ws, err := encodeWarnings(st.Warnings)
	if err != nil {
		return err
	}
	err = s.q(ctx).CreateStructuring(ctx, dbgen.CreateStructuringParams{
		VersionID: st.VersionID, State: string(st.State), Failure: string(st.Failure), SourceText: st.SourceText,
		Warnings: ws, Revision: int32(st.Revision), UpdatedBy: strPtr(st.UpdatedBy), UpdatedAt: st.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("criando estruturação: %w", err)
	}
	return nil
}

func (s *Structurings) FindByVersion(ctx context.Context, versionID string) (domain.Structuring, error) {
	return s.find(s.q(ctx).GetStructuring(ctx, versionID))
}

func (s *Structurings) FindByVersionForUpdate(ctx context.Context, versionID string) (domain.Structuring, error) {
	return s.find(s.q(ctx).GetStructuringForUpdate(ctx, versionID))
}

func (s *Structurings) find(row dbgen.PropositionStructuring, err error) (domain.Structuring, error) {
	if errors.Is(err, pgx.ErrNoRows) || isInvalidText(err) {
		return domain.Structuring{}, domain.ErrStructuringNotFound
	}
	if err != nil {
		return domain.Structuring{}, fmt.Errorf("lendo estruturação: %w", err)
	}
	return toStructuring(row)
}

func (s *Structurings) ListByVersions(ctx context.Context, versionIDs []string) ([]domain.Structuring, error) {
	if len(versionIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListStructurings(ctx, versionIDs)
	if err != nil {
		return nil, fmt.Errorf("listando estruturações: %w", err)
	}
	out := make([]domain.Structuring, 0, len(rows))
	for _, r := range rows {
		st, err := toStructuring(r)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *Structurings) Update(ctx context.Context, st domain.Structuring, prevRevision int) error {
	ws, err := encodeWarnings(st.Warnings)
	if err != nil {
		return err
	}
	n, err := s.q(ctx).UpdateStructuring(ctx, dbgen.UpdateStructuringParams{
		State: string(st.State), Failure: string(st.Failure), SourceText: st.SourceText, Warnings: ws,
		Revision: int32(st.Revision), UpdatedBy: strPtr(st.UpdatedBy), UpdatedAt: st.UpdatedAt,
		ConfirmedBy: strPtr(st.ConfirmedBy), ConfirmedAt: st.ConfirmedAt,
		VersionID: st.VersionID, PrevRevision: int32(prevRevision),
	})
	if err != nil {
		return fmt.Errorf("gravando estruturação: %w", err)
	}
	if n == 0 {
		return domain.ErrStaleContent
	}
	return nil
}

type provisionJSON struct {
	Address       string `json:"address"`
	ParentAddress string `json:"parent_address"`
	Kind          string `json:"kind"`
	Label         string `json:"label"`
	Text          string `json:"text"`
	Depth         int    `json:"depth"`
	Position      int    `json:"position"`
	TextHash      string `json:"text_hash"`
}

func (s *Structurings) ReplaceProvisions(ctx context.Context, versionID string, ps []domain.Provision) error {
	q := s.q(ctx)
	if err := q.DeleteProvisions(ctx, versionID); err != nil {
		return fmt.Errorf("apagando dispositivos: %w", err)
	}
	if len(ps) == 0 {
		return nil
	}
	rows := make([]provisionJSON, len(ps))
	for i, p := range ps {
		rows[i] = provisionJSON{
			Address: p.Address, ParentAddress: p.Parent, Kind: string(p.Kind), Label: p.Label, Text: p.Text,
			Depth: p.Depth, Position: p.Position, TextHash: p.TextHash,
		}
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	if err := q.InsertProvisions(ctx, dbgen.InsertProvisionsParams{VersionID: versionID, Rows: b}); err != nil {
		return fmt.Errorf("gravando dispositivos: %w", err)
	}
	return nil
}

func (s *Structurings) ListProvisions(ctx context.Context, versionID string) ([]domain.Provision, error) {
	rows, err := s.q(ctx).ListProvisions(ctx, versionID)
	if isInvalidText(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("listando dispositivos: %w", err)
	}
	out := make([]domain.Provision, len(rows))
	for i, r := range rows {
		out[i] = domain.Provision{
			VersionID: r.VersionID,
			Provision: structuring.Provision{
				Address: r.Address, Parent: r.ParentAddress, Kind: structuring.Kind(r.Kind),
				Label: r.Label, Text: r.Text, Depth: int(r.Depth),
			},
			Position: int(r.Position), TextHash: r.TextHash,
		}
	}
	return out, nil
}

func toStructuring(r dbgen.PropositionStructuring) (domain.Structuring, error) {
	var ws []warningJSON
	if err := json.Unmarshal(r.Warnings, &ws); err != nil {
		return domain.Structuring{}, fmt.Errorf("lendo avisos da estruturação: %w", err)
	}
	st := domain.Structuring{
		VersionID: r.VersionID, State: domain.StructuringState(r.State), Failure: domain.StructuringFailure(r.Failure),
		SourceText: r.SourceText, Revision: int(r.Revision), UpdatedAt: r.UpdatedAt.UTC(), ConfirmedAt: r.ConfirmedAt,
	}
	for _, w := range ws {
		st.Warnings = append(st.Warnings, structuring.Warning(w))
	}
	if r.UpdatedBy != nil {
		st.UpdatedBy = *r.UpdatedBy
	}
	if r.ConfirmedBy != nil {
		st.ConfirmedBy = *r.ConfirmedBy
	}
	if st.ConfirmedAt != nil {
		t := st.ConfirmedAt.UTC()
		st.ConfirmedAt = &t
	}
	return st, nil
}
