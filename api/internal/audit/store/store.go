// Package store é o adaptador Postgres da Auditoria.
package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/audit/domain"
	"github.com/mphennrichs/nelisa/api/internal/audit/store/dbgen"
	"github.com/mphennrichs/nelisa/api/internal/platform/db"
)

// Store implementa app.EventRepo. Usa a transação do contexto quando há uma
// (db.Conn), para gravar junto com a ação auditada.
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Store) Insert(ctx context.Context, e domain.Event) error {
	meta, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("serializando metadata: %w", err)
	}
	if e.Metadata == nil {
		meta = []byte("{}")
	}
	var actor *string
	if e.ActorID != "" {
		actor = &e.ActorID
	}
	return s.q(ctx).InsertEvent(ctx, dbgen.InsertEventParams{
		ActorID: actor, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID,
		At: e.At, Metadata: meta,
	})
}

// ListByTarget devolve os eventos de um alvo, do mais antigo ao mais novo.
func (s *Store) ListByTarget(ctx context.Context, targetType, targetID string) ([]domain.Event, error) {
	rows, err := s.q(ctx).ListEventsByTarget(ctx, dbgen.ListEventsByTargetParams{TargetType: targetType, TargetID: targetID})
	if err != nil {
		return nil, fmt.Errorf("listando eventos: %w", err)
	}
	out := make([]domain.Event, 0, len(rows))
	for _, r := range rows {
		e := domain.Event{Action: r.Action, TargetType: r.TargetType, TargetID: r.TargetID, At: r.At.UTC()}
		if r.ActorID != nil {
			e.ActorID = *r.ActorID
		}
		if err := json.Unmarshal(r.Metadata, &e.Metadata); err != nil {
			return nil, fmt.Errorf("lendo metadata: %w", err)
		}
		out = append(out, e)
	}
	return out, nil
}
