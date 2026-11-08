// Package fakes tem implementações em memória das portas de
// proposition/app, escritas à mão, usadas pelos testes de app/ e de
// httpapi/. A transação falsa guarda uma cópia do estado e a restaura se a
// função devolver erro, para os testes provarem que mudança, Auditoria e
// enfileiramento andam juntos. Os índices únicos do banco são imitados.
package fakes

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sort"
	"sync"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// RecordedEvent é um evento de Auditoria e se ele chegou numa transação.
type RecordedEvent struct {
	app.AuditEvent
	InTx bool
}

// World é o "banco" em memória.
type World struct {
	mu        sync.Mutex
	props     map[string]domain.Proposition
	links     map[string]domain.SourceLink
	events    []RecordedEvent
	scheduled []string
	timeline  map[string][]app.TimelineEntry
	versions  map[string]domain.Version
	contents  map[string]domain.Content
	topics    map[string]domain.Topic
	claims    map[string]app.ClaimInfo
	inTx      bool

	// Statuses e Badges são o que o módulo review responderia.
	Statuses map[string]app.ReviewStatus
	Badges   map[string]int
	// Errata são as Erratas aprovadas que o módulo review mandaria.
	Errata []domain.PublicErratum
	// Names são os nomes da equipe (módulo team).
	Names map[string]string

	// AuditErr, se definido, faz toda gravação de Auditoria falhar.
	AuditErr error
	// ScheduleErr, se definido, faz todo enfileiramento falhar.
	ScheduleErr error
}

func NewWorld() *World {
	return &World{
		props: map[string]domain.Proposition{}, links: map[string]domain.SourceLink{},
		timeline: map[string][]app.TimelineEntry{},
		versions: map[string]domain.Version{}, contents: map[string]domain.Content{}, topics: map[string]domain.Topic{},
		claims: map[string]app.ClaimInfo{}, Statuses: map[string]app.ReviewStatus{}, Badges: map[string]int{},
		Names: map[string]string{},
	}
}

func (w *World) Props() app.PropositionRepo   { return props{w} }
func (w *World) Links() app.LinkRepo          { return links{w} }
func (w *World) Tx() app.TxRunner             { return tx{w} }
func (w *World) Audit() app.AuditRecorder     { return audit{w} }
func (w *World) Scheduler() app.SyncScheduler { return scheduler{w} }
func (w *World) Timeline() app.TimelineReader { return timeline{w} }
func (w *World) Events() []RecordedEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.events)
}
func (w *World) Scheduled() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.scheduled)
}
func (w *World) Put(p domain.Proposition)    { w.mu.Lock(); defer w.mu.Unlock(); w.props[p.ID] = p }
func (w *World) PutLink(l domain.SourceLink) { w.mu.Lock(); defer w.mu.Unlock(); w.links[l.ID] = l }
func (w *World) SetTimeline(id string, es []app.TimelineEntry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.timeline[id] = es
}

// Get devolve a Proposição guardada (montagem e conferência de teste).
func (w *World) Get(id string) (domain.Proposition, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.props[id]
	return p, ok
}

// AllLinks devolve todos os Vínculos, ordenados por id.
func (w *World) AllLinks() []domain.SourceLink {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := slices.Collect(maps.Values(w.links))
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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
	props, links := maps.Clone(w.props), maps.Clone(w.links)
	events, scheduled := slices.Clone(w.events), slices.Clone(w.scheduled)
	versions, contents, topics := maps.Clone(w.versions), maps.Clone(w.contents), maps.Clone(w.topics)
	claims := maps.Clone(w.claims)
	w.inTx = true
	w.mu.Unlock()

	err := fn(ctx)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.inTx = false
	if err != nil {
		w.props, w.links, w.events, w.scheduled = props, links, events, scheduled
		w.versions, w.contents, w.topics, w.claims = versions, contents, topics, claims
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

type scheduler struct{ w *World }

func (s scheduler) ScheduleSync(_ context.Context, linkID string) error {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	if s.w.ScheduleErr != nil {
		return s.w.ScheduleErr
	}
	s.w.scheduled = append(s.w.scheduled, linkID)
	return nil
}

type timeline struct{ w *World }

func (t timeline) Timeline(_ context.Context, id string) ([]app.TimelineEntry, error) {
	t.w.mu.Lock()
	defer t.w.mu.Unlock()
	return slices.Clone(t.w.timeline[id]), nil
}

// --- Proposições ---

type props struct{ w *World }

func sortProps(ps []domain.Proposition) []domain.Proposition {
	sort.Slice(ps, func(i, j int) bool {
		if !ps[i].UpdatedAt.Equal(ps[j].UpdatedAt) {
			return ps[i].UpdatedAt.After(ps[j].UpdatedAt)
		}
		return ps[i].ID < ps[j].ID
	})
	return ps
}

func (r props) filter(keep func(domain.Proposition) bool) []domain.Proposition {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.Proposition
	for _, p := range r.w.props {
		if keep(p) {
			out = append(out, p)
		}
	}
	return sortProps(out)
}

func (r props) Create(_ context.Context, p domain.Proposition) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, o := range r.w.props {
		if o.Slug == p.Slug {
			return domain.ErrSlugTaken
		}
		if o.OriginChamber == p.OriginChamber && o.Kind == p.Kind && o.Number == p.Number && o.Year == p.Year {
			return domain.ErrPropositionExists
		}
	}
	r.w.props[p.ID] = p
	return nil
}

func (r props) FindByID(_ context.Context, id string) (domain.Proposition, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	p, ok := r.w.props[id]
	if !ok {
		return domain.Proposition{}, domain.ErrPropositionNotFound
	}
	return p, nil
}

func (r props) FindByIDForUpdate(ctx context.Context, id string) (domain.Proposition, error) {
	return r.FindByID(ctx, id)
}

func (r props) FindBySlug(_ context.Context, slug string) (domain.Proposition, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, p := range r.w.props {
		if p.Slug == slug {
			return p, nil
		}
	}
	return domain.Proposition{}, domain.ErrPropositionNotFound
}

func (r props) SlugExists(ctx context.Context, slug string) (bool, error) {
	_, err := r.FindBySlug(ctx, slug)
	return err == nil, nil
}

func (r props) FindByName(_ context.Context, kind string, number, year int) ([]domain.Proposition, error) {
	return r.filter(func(p domain.Proposition) bool { return p.Kind == kind && p.Number == number && p.Year == year }), nil
}

func (r props) List(context.Context) ([]domain.Proposition, error) {
	return r.filter(func(domain.Proposition) bool { return true }), nil
}

func (r props) ListInStates(_ context.Context, states []domain.ReviewState) ([]domain.Proposition, error) {
	return r.filter(func(p domain.Proposition) bool { return slices.Contains(states, p.ReviewState) }), nil
}

func (r props) ListAttachedTo(_ context.Context, id string) ([]domain.Proposition, error) {
	return r.filter(func(p domain.Proposition) bool { return p.AttachedToID != nil && *p.AttachedToID == id }), nil
}

func (r props) ListUnresolvedAttachments(_ context.Context, name string) ([]domain.Proposition, error) {
	return r.filter(func(p domain.Proposition) bool { return p.AttachedToID == nil && p.AttachedToName == name }), nil
}

func (r props) Update(_ context.Context, p domain.Proposition) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.props[p.ID]
	if !ok {
		return domain.ErrPropositionNotFound
	}
	// Só os campos que o UPDATE do store grava.
	cur.OfficialData, cur.StatusAt, cur.AttachedToID = p.OfficialData, p.StatusAt, p.AttachedToID
	cur.ReviewState, cur.UpdatedAt = p.ReviewState, p.UpdatedAt
	r.w.props[p.ID] = cur
	return nil
}

// --- Vínculos ---

type links struct{ w *World }

// uniqueErr imita os índices únicos de source_links para l, ignorando
// a própria linha.
func (r links) uniqueErr(l domain.SourceLink) error {
	for _, o := range r.w.links {
		if o.ID == l.ID {
			continue
		}
		if o.PropositionID == l.PropositionID && o.Chamber == l.Chamber && o.ExternalID == l.ExternalID {
			return domain.ErrLinkExists
		}
		if l.State == domain.LinkConfirmed && o.State == domain.LinkConfirmed {
			if o.Chamber == l.Chamber && o.ExternalID == l.ExternalID {
				return domain.ErrLinkTaken
			}
			if o.PropositionID == l.PropositionID && o.Chamber == l.Chamber {
				return domain.ErrChamberAlreadyLinked
			}
		}
	}
	return nil
}

func (r links) Create(_ context.Context, l domain.SourceLink) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if err := r.uniqueErr(l); err != nil {
		return err
	}
	r.w.links[l.ID] = l
	return nil
}

func (r links) FindByID(_ context.Context, id string) (domain.SourceLink, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	l, ok := r.w.links[id]
	if !ok {
		return domain.SourceLink{}, domain.ErrLinkNotFound
	}
	return l, nil
}

func (r links) filter(keep func(domain.SourceLink) bool) []domain.SourceLink {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.SourceLink
	for _, l := range r.w.links {
		if keep(l) {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Chamber != out[j].Chamber {
			return out[i].Chamber < out[j].Chamber
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (r links) ListByProposition(_ context.Context, id string) ([]domain.SourceLink, error) {
	return r.filter(func(l domain.SourceLink) bool { return l.PropositionID == id }), nil
}

func (r links) ListByPropositions(_ context.Context, ids []string) ([]domain.SourceLink, error) {
	return r.filter(func(l domain.SourceLink) bool { return slices.Contains(ids, l.PropositionID) }), nil
}

func (r links) ListByExternal(_ context.Context, c domain.Chamber, ids []string) ([]domain.SourceLink, error) {
	return r.filter(func(l domain.SourceLink) bool { return l.Chamber == c && slices.Contains(ids, l.ExternalID) }), nil
}

func (r links) ListConfirmedIDs(context.Context) ([]string, error) {
	var out []string
	for _, l := range r.filter(func(l domain.SourceLink) bool { return l.State == domain.LinkConfirmed }) {
		out = append(out, l.ID)
	}
	sort.Strings(out)
	return out, nil
}

func (r links) Confirm(_ context.Context, l domain.SourceLink) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.links[l.ID]
	if !ok || cur.State != domain.LinkSuggested {
		return domain.ErrLinkNotSuggested
	}
	cur.State, cur.ConfirmedBy, cur.ConfirmedAt = domain.LinkConfirmed, l.ConfirmedBy, l.ConfirmedAt
	if err := r.uniqueErr(cur); err != nil {
		return err
	}
	r.w.links[l.ID] = cur
	return nil
}

func (r links) DeleteSuggested(_ context.Context, id string) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.links[id]
	if !ok || cur.State != domain.LinkSuggested {
		return domain.ErrLinkNotSuggested
	}
	delete(r.w.links, id)
	return nil
}

func (r links) UpdateSync(_ context.Context, l domain.SourceLink) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.links[l.ID]
	if !ok {
		return domain.ErrLinkNotFound
	}
	cur.LastSyncedAt, cur.LastSyncError, cur.SourceName, cur.SourceURL = l.LastSyncedAt, l.LastSyncError, l.SourceName, l.SourceURL
	r.w.links[l.ID] = cur
	return nil
}

// ErrBoom é um erro qualquer para simular falha de infraestrutura.
var ErrBoom = errors.New("fakes: falha simulada")
