// Package fakes tem implementações em memória das portas de
// contribution/app, para os testes de app/ e de httpapi/. A transação
// falsa restaura o estado se a função devolver erro.
package fakes

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/mphennrichs/nelisa/api/internal/contribution/app"
	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
)

// RecordedEvent é um evento de Auditoria e se ele chegou numa transação.
type RecordedEvent struct {
	app.AuditEvent
	InTx bool
}

// World é o "banco" em memória.
type World struct {
	mu            sync.Mutex
	Contributions map[string]domain.Contribution // por id
	Sources       map[string]domain.OfficialSource
	Events        []RecordedEvent
	inTx          bool
	// Public são as Proposições públicas, por slug.
	Public map[string]app.PublicProposition
}

func New() *World {
	return &World{
		Contributions: map[string]domain.Contribution{},
		Sources:       map[string]domain.OfficialSource{},
		Public:        map[string]app.PublicProposition{},
	}
}

// --- ContributionRepo ---

type Contributions struct{ W *World }

func (r Contributions) Insert(_ context.Context, c domain.Contribution) error {
	r.W.mu.Lock()
	defer r.W.mu.Unlock()
	for _, o := range r.W.Contributions {
		if o.TrackingHash == c.TrackingHash {
			return app.ErrTrackingCodeTaken
		}
	}
	r.W.Contributions[c.ID] = c
	return nil
}

func (r Contributions) FindByTrackingHash(_ context.Context, hash string) (domain.Contribution, error) {
	r.W.mu.Lock()
	defer r.W.mu.Unlock()
	for _, c := range r.W.Contributions {
		if c.TrackingHash == hash {
			return c, nil
		}
	}
	return domain.Contribution{}, domain.ErrContributionNotFound
}

func (r Contributions) CountPending(context.Context) (int, error) {
	r.W.mu.Lock()
	defer r.W.mu.Unlock()
	n := 0
	for _, c := range r.W.Contributions {
		if c.State == domain.StatePending {
			n++
		}
	}
	return n, nil
}

// --- OfficialSourceRepo ---

type Sources struct{ W *World }

func (r Sources) List(context.Context) ([]domain.OfficialSource, error) {
	r.W.mu.Lock()
	defer r.W.mu.Unlock()
	out := slices.Collect(maps.Values(r.W.Sources))
	slices.SortFunc(out, func(a, b domain.OfficialSource) int {
		if a.Domain < b.Domain {
			return -1
		}
		return 1
	})
	return out, nil
}

func (r Sources) Domains(ctx context.Context) ([]string, error) {
	ss, _ := r.List(ctx)
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Domain)
	}
	return out, nil
}

func (r Sources) Insert(_ context.Context, s domain.OfficialSource) error {
	r.W.mu.Lock()
	defer r.W.mu.Unlock()
	for _, o := range r.W.Sources {
		if o.Domain == s.Domain {
			return domain.ErrOfficialSourceExists
		}
	}
	r.W.Sources[s.ID] = s
	return nil
}

func (r Sources) Delete(_ context.Context, id string) (domain.OfficialSource, error) {
	r.W.mu.Lock()
	defer r.W.mu.Unlock()
	s, ok := r.W.Sources[id]
	if !ok {
		return domain.OfficialSource{}, domain.ErrOfficialSourceNotFound
	}
	delete(r.W.Sources, id)
	return s, nil
}

// --- Propositions ---

type Props struct{ W *World }

func (p Props) PublicBySlug(_ context.Context, slug string) (*app.PublicProposition, error) {
	p.W.mu.Lock()
	defer p.W.mu.Unlock()
	if v, ok := p.W.Public[slug]; ok {
		return &v, nil
	}
	return nil, nil
}

func (p Props) PublicByID(_ context.Context, id string) (*app.PublicProposition, error) {
	p.W.mu.Lock()
	defer p.W.mu.Unlock()
	for _, v := range p.W.Public {
		if v.ID == id {
			return &v, nil
		}
	}
	return nil, nil
}

// --- Tx e Auditoria ---

type Tx struct{ W *World }

func (t Tx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	t.W.mu.Lock()
	contribs, sources, events := maps.Clone(t.W.Contributions), maps.Clone(t.W.Sources), slices.Clone(t.W.Events)
	t.W.inTx = true
	t.W.mu.Unlock()
	err := fn(ctx)
	t.W.mu.Lock()
	defer t.W.mu.Unlock()
	t.W.inTx = false
	if err != nil {
		t.W.Contributions, t.W.Sources, t.W.Events = contribs, sources, events
	}
	return err
}

type Audit struct{ W *World }

func (a Audit) Record(_ context.Context, e app.AuditEvent) error {
	a.W.mu.Lock()
	defer a.W.mu.Unlock()
	a.W.Events = append(a.W.Events, RecordedEvent{AuditEvent: e, InTx: a.W.inTx})
	return nil
}
