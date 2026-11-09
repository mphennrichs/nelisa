// Package fakes tem implementações em memória das portas de queue/app,
// escritas à mão, usadas pelos testes de app/ e de httpapi/. A transação
// falsa restaura Posses e eventos se a função devolver erro.
package fakes

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sort"
	"sync"

	"github.com/mphennrichs/nelisa/api/internal/queue/app"
	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
)

// ErrBoom é um erro qualquer para simular falha de infraestrutura.
var ErrBoom = errors.New("fakes: falha simulada")

// RecordedEvent é um evento de Auditoria e se ele chegou numa transação.
type RecordedEvent struct {
	app.AuditEvent
	InTx bool
}

// World é o "banco" em memória.
type World struct {
	mu     sync.Mutex
	claims map[string]domain.Claim
	events []RecordedEvent
	inTx   bool
	// Locks conta as chamadas a Lock (prova que quem mexe na Posse trava).
	Locks int

	// People é o cadastro da equipe (módulo team).
	People map[string]app.Person
	// InReview, Described e SecondReviews são o que os módulos
	// proposition e review responderiam.
	InReview      []app.Proposition
	Described     map[string]app.Proposition
	SecondReviews []app.PendingSecondReview
	Corrections   []app.PendingCorrection
	// Contributions são as Contribuições pendentes (módulo contribution).
	Contributions []app.PendingContribution
	// AuditErr, se definido, faz toda gravação de Auditoria falhar.
	AuditErr error
}

func NewWorld() *World {
	return &World{claims: map[string]domain.Claim{}, People: map[string]app.Person{}, Described: map[string]app.Proposition{}}
}

func (w *World) Claims() app.ClaimRepo   { return claims{w} }
func (w *World) Directory() app.People   { return people{w} }
func (w *World) Props() app.Propositions { return props{w} }
func (w *World) Reviews() app.ReviewWork { return reviews{w} }
func (w *World) Contribs() app.Contributions {
	return contribs{w}
}
func (w *World) Tx() app.TxRunner         { return tx{w} }
func (w *World) Audit() app.AuditRecorder { return audit{w} }
func (w *World) PutClaim(c domain.Claim) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.claims[c.PropositionID] = c
}
func (w *World) Claim(id string) (domain.Claim, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.claims[id]
	return c, ok
}
func (w *World) Events() []RecordedEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.events)
}

type tx struct{ w *World }

func (t tx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	w := t.w
	w.mu.Lock()
	if w.inTx {
		w.mu.Unlock()
		return fn(ctx)
	}
	claims, events := maps.Clone(w.claims), slices.Clone(w.events)
	w.inTx = true
	w.mu.Unlock()
	err := fn(ctx)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.inTx = false
	if err != nil {
		w.claims, w.events = claims, events
	}
	return err
}

type audit struct{ w *World }

func (a audit) Record(_ context.Context, e app.AuditEvent) error {
	a.w.mu.Lock()
	defer a.w.mu.Unlock()
	if a.w.AuditErr != nil {
		return a.w.AuditErr
	}
	a.w.events = append(a.w.events, RecordedEvent{AuditEvent: e, InTx: a.w.inTx})
	return nil
}

type claims struct{ w *World }

func (c claims) Lock(context.Context, string) error {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	c.w.Locks++
	return nil
}

func (c claims) Find(_ context.Context, id string) (*domain.Claim, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	cl, ok := c.w.claims[id]
	if !ok {
		return nil, nil
	}
	return &cl, nil
}

func (c claims) ListByPropositions(_ context.Context, ids []string) ([]domain.Claim, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	var out []domain.Claim
	for _, id := range ids {
		if cl, ok := c.w.claims[id]; ok {
			out = append(out, cl)
		}
	}
	return out, nil
}

func (c claims) Save(_ context.Context, cl domain.Claim) error {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	c.w.claims[cl.PropositionID] = cl
	return nil
}

func (c claims) Delete(_ context.Context, id string) error {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	delete(c.w.claims, id)
	return nil
}

type people struct{ w *World }

func (p people) Lookup(_ context.Context, ids []string) (map[string]app.Person, error) {
	p.w.mu.Lock()
	defer p.w.mu.Unlock()
	out := map[string]app.Person{}
	for _, id := range ids {
		if v, ok := p.w.People[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

type props struct{ w *World }

func (p props) Exists(_ context.Context, id string) (bool, error) {
	p.w.mu.Lock()
	defer p.w.mu.Unlock()
	if _, ok := p.w.Described[id]; ok {
		return true, nil
	}
	for _, q := range p.w.InReview {
		if q.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (p props) InReview(context.Context) ([]app.Proposition, error) {
	p.w.mu.Lock()
	defer p.w.mu.Unlock()
	return slices.Clone(p.w.InReview), nil
}

func (p props) Describe(_ context.Context, ids []string) ([]app.Proposition, error) {
	p.w.mu.Lock()
	defer p.w.mu.Unlock()
	var out []app.Proposition
	for _, id := range ids {
		if q, ok := p.w.Described[id]; ok {
			out = append(out, q)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

type reviews struct{ w *World }

func (r reviews) PendingSecondReviews(context.Context) ([]app.PendingSecondReview, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	return slices.Clone(r.w.SecondReviews), nil
}

func (r reviews) PendingCorrections(context.Context) ([]app.PendingCorrection, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	return slices.Clone(r.w.Corrections), nil
}

type contribs struct{ w *World }

func (c contribs) Pending(context.Context) ([]app.PendingContribution, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	return slices.Clone(c.w.Contributions), nil
}
