// Package fakes tem implementações em memória das portas de review/app,
// escritas à mão, usadas pelos testes de app/ e de httpapi/. A transação
// falsa guarda uma cópia do estado (aprovações, Versões, Posses, eventos)
// e a restaura se a função devolver erro, para os testes provarem que
// aprovação, promoção da Versão, Posse e Auditoria andam juntas.
package fakes

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sort"
	"sync"

	"github.com/mphennrichs/nelisa/api/internal/review/app"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

// ErrBoom é um erro qualquer para simular falha de infraestrutura.
var ErrBoom = errors.New("fakes: falha simulada")

// RecordedEvent é um evento de Auditoria e se ele chegou numa transação.
type RecordedEvent struct {
	app.AuditEvent
	InTx bool
}

// Version é uma Versão como o módulo proposition a guardaria.
type Version struct {
	ID            string
	PropositionID string
	Seq           int
	// State: "pending", "current" ou "superseded".
	State    string
	Revision int
	Authors  []string
	Missing  []domain.Blocker
}

// Proposition é o que o fake sabe da Proposição.
type Proposition struct {
	ID    string
	Name  string
	State string // "draft", "in_review", "ready"
}

// World é o "banco" em memória.
type World struct {
	mu        sync.Mutex
	approvals map[string]domain.Approval
	versions  map[string]Version
	props     map[string]Proposition
	claims    map[string]string
	events    []RecordedEvent
	inTx      bool

	// AuditErr, se definido, faz toda gravação de Auditoria falhar.
	AuditErr error
}

func NewWorld() *World {
	return &World{
		approvals: map[string]domain.Approval{}, versions: map[string]Version{},
		props: map[string]Proposition{}, claims: map[string]string{},
	}
}

func (w *World) Approvals() app.ApprovalRepo { return approvals{w} }
func (w *World) Editorial() app.Editorial    { return editorial{w} }
func (w *World) Claims() app.Claims          { return claims{w} }
func (w *World) Tx() app.TxRunner            { return tx{w} }
func (w *World) Audit() app.AuditRecorder    { return audit{w} }

// PutProposition, PutVersion, PutApproval e SetClaim montam o cenário.
func (w *World) PutProposition(p Proposition) { w.mu.Lock(); defer w.mu.Unlock(); w.props[p.ID] = p }
func (w *World) PutVersion(v Version)         { w.mu.Lock(); defer w.mu.Unlock(); w.versions[v.ID] = v }
func (w *World) PutApproval(a domain.Approval) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.approvals[a.VersionID] = a
}
func (w *World) SetClaim(propositionID, userID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.claims[propositionID] = userID
}

// Snapshot devolve o estado para conferência.
func (w *World) Proposition(id string) Proposition {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.props[id]
}
func (w *World) Version(id string) Version { w.mu.Lock(); defer w.mu.Unlock(); return w.versions[id] }
func (w *World) Approval(versionID string) (domain.Approval, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	a, ok := w.approvals[versionID]
	return a, ok
}
func (w *World) ClaimOf(propositionID string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.claims[propositionID]
}
func (w *World) Events() []RecordedEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.events)
}

// --- transação ---

type tx struct{ w *World }

func (t tx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	w := t.w
	w.mu.Lock()
	if w.inTx {
		w.mu.Unlock()
		return fn(ctx)
	}
	approvals, versions, props, claims := maps.Clone(w.approvals), maps.Clone(w.versions), maps.Clone(w.props), maps.Clone(w.claims)
	events := slices.Clone(w.events)
	w.inTx = true
	w.mu.Unlock()

	err := fn(ctx)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.inTx = false
	if err != nil {
		w.approvals, w.versions, w.props, w.claims, w.events = approvals, versions, props, claims, events
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

// --- aprovações ---

type approvals struct{ w *World }

func (r approvals) Create(_ context.Context, a domain.Approval) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if _, ok := r.w.approvals[a.VersionID]; ok {
		return domain.ErrAlreadyApproved
	}
	r.w.approvals[a.VersionID] = a
	return nil
}

func (r approvals) FindByVersion(_ context.Context, id string) (domain.Approval, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	a, ok := r.w.approvals[id]
	if !ok {
		return domain.Approval{}, domain.ErrApprovalNotFound
	}
	return a, nil
}

func (r approvals) FindByVersionForUpdate(ctx context.Context, id string) (domain.Approval, error) {
	return r.FindByVersion(ctx, id)
}

func (r approvals) ListByVersions(_ context.Context, ids []string) ([]domain.Approval, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.Approval
	for _, id := range ids {
		if a, ok := r.w.approvals[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r approvals) ListPendingSecondReview(context.Context) ([]domain.Approval, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.Approval
	for _, a := range r.w.approvals {
		if a.PendingSecondReview() {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ApprovedAt.Before(out[j].ApprovedAt) })
	return out, nil
}

func (r approvals) SetSecondReview(_ context.Context, a domain.Approval) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.approvals[a.VersionID]
	if !ok || !cur.PendingSecondReview() {
		return domain.ErrNoSecondReviewPending
	}
	cur.SecondReviewBy, cur.SecondReviewAt = a.SecondReviewBy, a.SecondReviewAt
	r.w.approvals[a.VersionID] = cur
	return nil
}

// --- ciclo editorial (o que o módulo proposition faria) ---

type editorial struct{ w *World }

func (e editorial) subject(p Proposition, v Version) app.Subject {
	return app.Subject{
		PropositionID: p.ID, Name: p.Name, InReview: p.State == "in_review", VersionID: v.ID, VersionSeq: v.Seq, Revision: v.Revision,
		Authors: slices.Clone(v.Authors), Missing: slices.Clone(v.Missing),
	}
}

func (e editorial) find(propositionID, state string) (Version, bool) {
	for _, v := range e.w.versions {
		if v.PropositionID == propositionID && v.State == state {
			return v, true
		}
	}
	return Version{}, false
}

func (e editorial) Pending(_ context.Context, propositionID string) (*app.Subject, error) {
	e.w.mu.Lock()
	defer e.w.mu.Unlock()
	p, ok := e.w.props[propositionID]
	if !ok {
		return nil, app.ErrPropositionNotFound
	}
	v, ok := e.find(propositionID, "pending")
	if !ok {
		return nil, nil
	}
	s := e.subject(p, v)
	return &s, nil
}

func (e editorial) LockPending(_ context.Context, propositionID, versionID string) (app.Subject, error) {
	e.w.mu.Lock()
	defer e.w.mu.Unlock()
	p, ok := e.w.props[propositionID]
	if !ok {
		return app.Subject{}, app.ErrPropositionNotFound
	}
	v, ok := e.w.versions[versionID]
	if !ok || v.PropositionID != propositionID {
		return app.Subject{}, app.ErrVersionNotFound
	}
	if v.State != "pending" {
		return app.Subject{}, app.ErrVersionNotPending
	}
	return e.subject(p, v), nil
}

func (e editorial) Promote(_ context.Context, propositionID, versionID string) error {
	e.w.mu.Lock()
	defer e.w.mu.Unlock()
	p := e.w.props[propositionID]
	if p.State != "in_review" {
		return domain.ErrNotInReview
	}
	if cur, ok := e.find(propositionID, "current"); ok {
		cur.State = "superseded"
		e.w.versions[cur.ID] = cur
	}
	v := e.w.versions[versionID]
	v.State = "current"
	e.w.versions[versionID] = v
	p.State = "ready"
	e.w.props[propositionID] = p
	return nil
}

func (e editorial) Version(_ context.Context, propositionID, versionID string) (app.VersionInfo, error) {
	e.w.mu.Lock()
	defer e.w.mu.Unlock()
	v, ok := e.w.versions[versionID]
	if !ok || v.PropositionID != propositionID {
		return app.VersionInfo{}, app.ErrVersionNotFound
	}
	return app.VersionInfo{PropositionID: propositionID, VersionID: v.ID, Seq: v.Seq, Current: v.State == "current", Authors: slices.Clone(v.Authors)}, nil
}

func (e editorial) CurrentVersionID(_ context.Context, propositionID string) (string, error) {
	e.w.mu.Lock()
	defer e.w.mu.Unlock()
	if _, ok := e.w.props[propositionID]; !ok {
		return "", app.ErrPropositionNotFound
	}
	v, _ := e.find(propositionID, "current")
	return v.ID, nil
}

// --- Posse ---

type claims struct{ w *World }

func (c claims) Release(_ context.Context, propositionID string) error {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	delete(c.w.claims, propositionID)
	return nil
}
