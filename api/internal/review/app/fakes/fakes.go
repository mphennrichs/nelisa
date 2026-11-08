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
	"strings"
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
	// Content é o conteúdo (Correção menor e Errata mexem nele).
	Content domain.Snapshot
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
	fixes     map[string]domain.MinorFix
	errata    map[string]domain.Erratum
	events    []RecordedEvent
	inTx      bool
	// Names dá o nome de quem tem a Posse (para o 409 claim_taken).
	Names map[string]string

	// AuditErr, se definido, faz toda gravação de Auditoria falhar.
	AuditErr error
}

func NewWorld() *World {
	return &World{
		approvals: map[string]domain.Approval{}, versions: map[string]Version{},
		props: map[string]Proposition{}, claims: map[string]string{},
		fixes: map[string]domain.MinorFix{}, errata: map[string]domain.Erratum{}, Names: map[string]string{},
	}
}

func (w *World) Approvals() app.ApprovalRepo  { return approvals{w} }
func (w *World) Editorial() app.Editorial     { return editorial{w} }
func (w *World) Claims() app.Claims           { return claims{w} }
func (w *World) Tx() app.TxRunner             { return tx{w} }
func (w *World) Audit() app.AuditRecorder     { return audit{w} }
func (w *World) Fixes() app.MinorFixRepo      { return fixes{w} }
func (w *World) Errata() app.ErratumRepo      { return errata{w} }
func (w *World) Corrections() app.Corrections { return corrections{w} }
func (w *World) ClaimGate() app.ClaimGate     { return claims{w} }

// PutMinorFix e PutErratum montam o cenário.
func (w *World) PutMinorFix(f domain.MinorFix) { w.mu.Lock(); defer w.mu.Unlock(); w.fixes[f.ID] = f }
func (w *World) PutErratum(e domain.Erratum)   { w.mu.Lock(); defer w.mu.Unlock(); w.errata[e.ID] = e }
func (w *World) MinorFix(id string) (domain.MinorFix, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, ok := w.fixes[id]
	return f, ok
}
func (w *World) Erratum(id string) (domain.Erratum, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	e, ok := w.errata[id]
	return e, ok
}
func (w *World) MinorFixes() []domain.MinorFix {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := slices.Collect(maps.Values(w.fixes))
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

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
	fixes, errata := maps.Clone(w.fixes), maps.Clone(w.errata)
	events := slices.Clone(w.events)
	w.inTx = true
	w.mu.Unlock()

	err := fn(ctx)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.inTx = false
	if err != nil {
		w.approvals, w.versions, w.props, w.claims, w.events = approvals, versions, props, claims, events
		w.fixes, w.errata = fixes, errata
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

func (c claims) TouchForEdit(_ context.Context, propositionID, userID string) error {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	if holder, ok := c.w.claims[propositionID]; ok && holder != userID {
		return &app.ClaimTakenError{HolderName: c.w.Names[holder]}
	}
	c.w.claims[propositionID] = userID
	return nil
}

// --- Correções menores ---

type fixes struct{ w *World }

func (r fixes) Create(_ context.Context, f domain.MinorFix) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.fixes[f.ID] = f
	return nil
}

func (r fixes) FindForUpdate(_ context.Context, id string) (domain.MinorFix, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	f, ok := r.w.fixes[id]
	if !ok {
		return domain.MinorFix{}, domain.ErrMinorFixNotFound
	}
	return f, nil
}

func (r fixes) SetChecked(_ context.Context, f domain.MinorFix) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.fixes[f.ID]
	if !ok || !cur.Unchecked() {
		return domain.ErrMinorFixChecked
	}
	cur.CheckedBy, cur.CheckedAt = f.CheckedBy, f.CheckedAt
	r.w.fixes[f.ID] = cur
	return nil
}

func (r fixes) list(keep func(domain.MinorFix) bool) []domain.MinorFix {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.MinorFix
	for _, f := range r.w.fixes {
		if f.Unchecked() && keep(f) {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (r fixes) ListUncheckedByProposition(_ context.Context, id string) ([]domain.MinorFix, error) {
	return r.list(func(f domain.MinorFix) bool { return f.PropositionID == id }), nil
}

func (r fixes) ListUnchecked(context.Context) ([]domain.MinorFix, error) {
	return r.list(func(domain.MinorFix) bool { return true }), nil
}

// --- Erratas ---

type errata struct{ w *World }

func (r errata) Create(_ context.Context, e domain.Erratum) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, o := range r.w.errata {
		if o.VersionID == e.VersionID && o.Pending() {
			return domain.ErrErratumPending
		}
	}
	r.w.errata[e.ID] = e
	return nil
}

func (r errata) FindForUpdate(_ context.Context, id string) (domain.Erratum, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	e, ok := r.w.errata[id]
	if !ok {
		return domain.Erratum{}, domain.ErrErratumNotFound
	}
	return e, nil
}

func (r errata) Decide(_ context.Context, e domain.Erratum) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.errata[e.ID]
	if !ok || !cur.Pending() {
		return domain.ErrErratumNotPending
	}
	r.w.errata[e.ID] = e
	return nil
}

func (r errata) HasPending(_ context.Context, versionID string) (bool, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, e := range r.w.errata {
		if e.VersionID == versionID && e.Pending() {
			return true, nil
		}
	}
	return false, nil
}

func (r errata) list(keep func(domain.Erratum) bool, newestFirst bool) []domain.Erratum {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.Erratum
	for _, e := range r.w.errata {
		if keep(e) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if newestFirst {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (r errata) ListByProposition(_ context.Context, id string) ([]domain.Erratum, error) {
	return r.list(func(e domain.Erratum) bool { return e.PropositionID == id }, true), nil
}

func (r errata) ListPending(context.Context) ([]domain.Erratum, error) {
	return r.list(domain.Erratum.Pending, false), nil
}

func (r errata) ListApprovedByVersions(_ context.Context, ids []string) ([]domain.Erratum, error) {
	return r.list(func(e domain.Erratum) bool {
		return e.State == domain.ErratumApproved && slices.Contains(ids, e.VersionID)
	}, true), nil
}

// --- conteúdo vigente (o que o módulo proposition faria) ---

type corrections struct{ w *World }

func (c corrections) target(p Proposition, v Version) domain.Target {
	return domain.Target{
		PropositionID: p.ID, Name: p.Name, Ready: p.State == "ready", VersionID: v.ID, Seq: v.Seq,
		Content: v.Content, Revision: v.Revision,
	}
}

func (c corrections) Current(_ context.Context, propositionID string) (*domain.Target, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	p, ok := c.w.props[propositionID]
	if !ok {
		return nil, app.ErrPropositionNotFound
	}
	v, ok := editorial(c).find(propositionID, "current")
	if !ok {
		return nil, nil
	}
	t := c.target(p, v)
	return &t, nil
}

func (c corrections) LockCurrent(_ context.Context, propositionID, versionID string) (domain.Target, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	p, ok := c.w.props[propositionID]
	if !ok {
		return domain.Target{}, app.ErrPropositionNotFound
	}
	v, ok := c.w.versions[versionID]
	if !ok || v.PropositionID != propositionID {
		return domain.Target{}, app.ErrVersionNotFound
	}
	if v.State != "current" {
		return domain.Target{}, domain.ErrVersionNotCurrent
	}
	return c.target(p, v), nil
}

// CheckContent imita as validações do conteúdo publicado: Resumo e Resumo
// curto não vazios e de 1 a 3 Temas.
func (c corrections) CheckContent(_ context.Context, s domain.Snapshot, _ []string) (domain.Snapshot, error) {
	s.Summary, s.ShortSummary = strings.TrimSpace(s.Summary), strings.TrimSpace(s.ShortSummary)
	if s.Summary == "" || s.ShortSummary == "" || len(s.TopicIDs) == 0 || len(s.TopicIDs) > 3 {
		return domain.Snapshot{}, &app.InvalidContentError{Code: "published_content_incomplete", Message: "incompleto"}
	}
	return s, nil
}

func (c corrections) Rewrite(_ context.Context, propositionID, versionID string, s domain.Snapshot, authorID string, revision int) (app.Rewrite, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	v, ok := c.w.versions[versionID]
	if !ok || v.PropositionID != propositionID {
		return app.Rewrite{}, app.ErrVersionNotFound
	}
	if v.State != "current" {
		return app.Rewrite{}, domain.ErrVersionNotCurrent
	}
	if v.Revision != revision {
		return app.Rewrite{}, domain.ErrStaleContent
	}
	var fields []string
	if v.Content.Summary != s.Summary {
		fields = append(fields, "summary")
	}
	if v.Content.ShortSummary != s.ShortSummary {
		fields = append(fields, "short_summary")
	}
	if !slices.Equal(v.Content.TopicIDs, s.TopicIDs) {
		fields = append(fields, "topic_ids")
	}
	if len(fields) == 0 {
		return app.Rewrite{Content: v.Content, Revision: v.Revision}, nil
	}
	v.Content = s
	v.Revision++
	if !slices.Contains(v.Authors, authorID) {
		v.Authors = append(slices.Clone(v.Authors), authorID)
	}
	c.w.versions[versionID] = v
	return app.Rewrite{Fields: fields, Content: s, Revision: v.Revision}, nil
}
