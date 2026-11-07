// Package fakes tem a implementação em memória da porta de progress/app,
// escrita à mão, com o mesmo índice único do banco (Proposição, Casa,
// chave).
package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/progress/domain"
)

type key struct{ prop, chamber, ext string }

// Entries implementa app.EntryRepo.
type Entries struct {
	mu   sync.Mutex
	rows map[key]domain.Entry
}

func NewEntries() *Entries { return &Entries{rows: map[key]domain.Entry{}} }

func (f *Entries) Upsert(_ context.Context, es []domain.Entry, _ time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, e := range es {
		k := key{e.PropositionID, e.Chamber, e.ExternalKey}
		if _, ok := f.rows[k]; !ok {
			n++
		}
		f.rows[k] = e
	}
	return n, nil
}

func (f *Entries) ListByProposition(_ context.Context, id string) ([]domain.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Entry
	for k, e := range f.rows {
		if k.prop == id {
			out = append(out, e)
		}
	}
	return out, nil
}

// Len é o total de eventos guardados.
func (f *Entries) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}
