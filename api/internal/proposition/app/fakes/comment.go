package fakes

import (
	"context"
	"slices"
	"sort"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

func (w *World) Comments() app.CommentRepo { return comments{w} }

// CommentsOf lê direto, inclusive apagados (asserção de teste).
func (w *World) CommentsOf(versionID string) []domain.Comment {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []domain.Comment
	for _, c := range w.comments {
		if c.VersionID == versionID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) || out[i].ID < out[j].ID })
	return out
}

type comments struct{ w *World }

func (r comments) Create(_ context.Context, c domain.Comment) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.comments[c.ID] = c
	return nil
}

func (r comments) FindForUpdate(_ context.Context, id string) (domain.Comment, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	c, ok := r.w.comments[id]
	if !ok {
		return domain.Comment{}, domain.ErrCommentNotFound
	}
	return c, nil
}

func (r comments) ListByVersions(_ context.Context, ids []string) ([]domain.Comment, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.Comment
	for _, c := range r.w.comments {
		if slices.Contains(ids, c.VersionID) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProvisionAddress != out[j].ProvisionAddress {
			return out[i].ProvisionAddress < out[j].ProvisionAddress
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt) || (out[i].CreatedAt.Equal(out[j].CreatedAt) && out[i].ID < out[j].ID)
	})
	return out, nil
}

func (r comments) Update(_ context.Context, c domain.Comment, prev int) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.comments[c.ID]
	if !ok {
		return domain.ErrCommentNotFound
	}
	if cur.Revision != prev {
		return domain.ErrStaleContent
	}
	r.w.comments[c.ID] = c
	return nil
}
