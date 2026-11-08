package fakes

import (
	"context"
	"slices"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

func (w *World) Structurings() app.StructuringRepo              { return structurings{w} }
func (w *World) StructuringScheduler() app.StructuringScheduler { return structSched{w} }

// StructuringQueue são as Versões com busca do PDF enfileirada.
func (w *World) StructuringQueue() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.structQueue)
}

// StructuringOf e ProvisionsOf leem direto (asserção de teste).
func (w *World) StructuringOf(versionID string) (domain.Structuring, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	s, ok := w.structurings[versionID]
	return s, ok
}

func (w *World) ProvisionsOf(versionID string) []domain.Provision {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.provisions[versionID])
}

// PutStructuring grava direto (montagem de teste).
func (w *World) PutStructuring(s domain.Structuring) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.structurings[s.VersionID] = s
}

type structurings struct{ w *World }

func (r structurings) Create(_ context.Context, s domain.Structuring) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.structurings[s.VersionID] = s
	return nil
}

func (r structurings) FindByVersion(_ context.Context, id string) (domain.Structuring, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	s, ok := r.w.structurings[id]
	if !ok {
		return domain.Structuring{}, domain.ErrStructuringNotFound
	}
	return s, nil
}

func (r structurings) FindByVersionForUpdate(ctx context.Context, id string) (domain.Structuring, error) {
	return r.FindByVersion(ctx, id)
}

func (r structurings) ListByVersions(_ context.Context, ids []string) ([]domain.Structuring, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.Structuring
	for _, id := range ids {
		if s, ok := r.w.structurings[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r structurings) Update(_ context.Context, s domain.Structuring, prev int) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.structurings[s.VersionID]
	if !ok {
		return domain.ErrStructuringNotFound
	}
	if cur.Revision != prev {
		return domain.ErrStaleContent
	}
	r.w.structurings[s.VersionID] = s
	return nil
}

func (r structurings) ReplaceProvisions(_ context.Context, id string, ps []domain.Provision) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.provisions[id] = slices.Clone(ps)
	return nil
}

func (r structurings) ListProvisions(_ context.Context, id string) ([]domain.Provision, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	return slices.Clone(r.w.provisions[id]), nil
}

type structSched struct{ w *World }

func (s structSched) ScheduleStructuring(_ context.Context, versionID string) error {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	if s.w.ScheduleErr != nil {
		return s.w.ScheduleErr
	}
	s.w.structQueue = append(s.w.structQueue, versionID)
	return nil
}
