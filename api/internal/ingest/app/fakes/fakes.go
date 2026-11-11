// Package fakes tem implementações em memória das portas de ingest/app e
// uma fonte oficial falsa, escritas à mão, usadas pelos testes de app/,
// httpapi/ e jobs/. A transação falsa restaura o estado se a função
// devolver erro, para os testes provarem que falha não deixa dado parcial.
package fakes

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/ingest/app"
)

// --- fonte oficial ---

// Source é uma fonte oficial falsa e configurável.
type Source struct {
	mu       sync.Mutex
	chamber  app.ChamberCode
	records  map[string]app.SourceRecord
	progress map[string][]app.SourceProgress

	// FetchErr, ProgressErr e SearchErr, se definidos, fazem a chamada
	// correspondente falhar.
	FetchErr    error
	ProgressErr error
	SearchErr   error
	// Calls conta as chamadas por método.
	Calls map[string]int

	// AgendaHits e PresentedHits são o que o Radar acha; RadarErr faz as
	// duas chamadas falharem.
	AgendaHits    []app.RadarHit
	PresentedHits []app.RadarHit
	RadarErr      error
}

func (s *Source) Agenda(context.Context, time.Time, time.Time) ([]app.RadarHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls["Agenda"]++
	return slices.Clone(s.AgendaHits), s.RadarErr
}

func (s *Source) Presented(context.Context, time.Time, time.Time) ([]app.RadarHit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls["Presented"]++
	if s.RadarErr != nil {
		return nil, s.RadarErr
	}
	return slices.Clone(s.PresentedHits), nil
}

func NewSource(c app.ChamberCode) *Source {
	return &Source{
		chamber: c, records: map[string]app.SourceRecord{}, progress: map[string][]app.SourceProgress{},
		Calls: map[string]int{},
	}
}

// Put cadastra um registro e sua Tramitação na fonte.
func (s *Source) Put(r app.SourceRecord, ps ...app.SourceProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.Chamber = s.chamber
	s.records[r.ExternalID] = r
	s.progress[r.ExternalID] = ps
}

func (s *Source) Chamber() app.ChamberCode { return s.chamber }

// Search devolve ErrNotFound embrulhado quando nada bate, como os
// adaptadores reais.
func (s *Source) Search(_ context.Context, q app.SearchQuery) ([]app.SourceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls["search"]++
	if s.SearchErr != nil {
		return nil, s.SearchErr
	}
	var out []app.SourceRecord
	for _, r := range s.records {
		if r.Kind == q.Kind && r.Number == q.Number && r.Year == q.Year {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fake %s: %w", s.chamber, app.ErrNotFound)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExternalID < out[j].ExternalID })
	return out, nil
}

func (s *Source) Fetch(_ context.Context, id string) (app.SourceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls["fetch"]++
	if s.FetchErr != nil {
		return app.SourceRecord{}, s.FetchErr
	}
	r, ok := s.records[id]
	if !ok {
		return app.SourceRecord{}, fmt.Errorf("fake %s/%s: %w", s.chamber, id, app.ErrNotFound)
	}
	return r, nil
}

func (s *Source) Progress(_ context.Context, id string) ([]app.SourceProgress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls["progress"]++
	if s.ProgressErr != nil {
		return nil, s.ProgressErr
	}
	return slices.Clone(s.progress[id]), nil
}

// Unavailable é o erro de uma fonte fora do ar.
func Unavailable() error { return fmt.Errorf("fake: HTTP 503: %w", app.ErrUnavailable) }

// --- "banco" ---

// Proposition é o que o Catalog falso guarda de uma Proposição.
type Proposition struct {
	ID      string
	Name    string
	Origin  app.ChamberCode
	Record  app.SourceRecord
	Syncs   int
	Editor  string
	Updated app.SourceRecord
}

// Link é um Vínculo no Catalog falso.
type Link struct {
	app.LinkRef
	LastError string
	Synced    int
}

type progressKey struct{ prop, chamber, key string }

// RecordedEvent é um evento de Auditoria e se chegou numa transação.
type RecordedEvent struct {
	app.AuditEvent
	InTx bool
}

// World guarda o estado das portas falsas.
type World struct {
	mu        sync.Mutex
	props     map[string]Proposition
	links     map[string]Link
	progress  map[progressKey]app.SourceProgress
	events    []RecordedEvent
	scheduled []string
	inTx      bool
	seq       int

	// AuditErr faz toda gravação de Auditoria falhar.
	AuditErr error
	// ProgressErr faz toda gravação de Tramitação falhar.
	ProgressErr error
}

func NewWorld() *World {
	return &World{props: map[string]Proposition{}, links: map[string]Link{}, progress: map[progressKey]app.SourceProgress{}}
}

func (w *World) Catalog() app.Catalog         { return catalog{w} }
func (w *World) Progress() app.ProgressWriter { return progress{w} }
func (w *World) Tx() app.TxRunner             { return tx{w} }
func (w *World) Audit() app.AuditRecorder     { return audit{w} }
func (w *World) Scheduler() app.SyncScheduler { return sched{w} }
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
func (w *World) ProgressCount() int         { w.mu.Lock(); defer w.mu.Unlock(); return len(w.progress) }
func (w *World) PropositionCount() int      { w.mu.Lock(); defer w.mu.Unlock(); return len(w.props) }
func (w *World) Prop(id string) Proposition { w.mu.Lock(); defer w.mu.Unlock(); return w.props[id] }
func (w *World) LinkByID(id string) Link    { w.mu.Lock(); defer w.mu.Unlock(); return w.links[id] }
func (w *World) PutLink(l Link)             { w.mu.Lock(); defer w.mu.Unlock(); w.links[l.ID] = l }

// Links devolve todos os Vínculos, ordenados por id.
func (w *World) Links() []Link {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := slices.Collect(maps.Values(w.links))
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (w *World) nextID(prefix string) string {
	w.seq++
	return fmt.Sprintf("%s-%d", prefix, w.seq)
}

type tx struct{ w *World }

func (t tx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	w := t.w
	w.mu.Lock()
	if w.inTx {
		w.mu.Unlock()
		return fn(ctx)
	}
	props, links, prog := maps.Clone(w.props), maps.Clone(w.links), maps.Clone(w.progress)
	events, scheduled := slices.Clone(w.events), slices.Clone(w.scheduled)
	w.inTx = true
	w.mu.Unlock()

	err := fn(ctx)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.inTx = false
	if err != nil {
		w.props, w.links, w.progress, w.events, w.scheduled = props, links, prog, events, scheduled
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

type sched struct{ w *World }

func (s sched) ScheduleSyncs(_ context.Context, ids []string) error {
	s.w.mu.Lock()
	defer s.w.mu.Unlock()
	s.w.scheduled = append(s.w.scheduled, ids...)
	return nil
}

type progress struct{ w *World }

func (p progress) Record(_ context.Context, propID string, c app.ChamberCode, es []app.SourceProgress) (int, error) {
	p.w.mu.Lock()
	defer p.w.mu.Unlock()
	if p.w.ProgressErr != nil {
		return 0, p.w.ProgressErr
	}
	n := 0
	for _, e := range es {
		k := progressKey{propID, string(c), e.ExternalKey}
		if _, ok := p.w.progress[k]; !ok {
			n++
		}
		p.w.progress[k] = e
	}
	return n, nil
}

type catalog struct{ w *World }

func (c catalog) LinksByExternal(_ context.Context, ch app.ChamberCode, ids []string) ([]app.LinkRef, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	var out []app.LinkRef
	for _, l := range c.w.links {
		if l.Chamber == ch && slices.Contains(ids, l.ExternalID) {
			out = append(out, l.LinkRef)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (c catalog) CreateImported(_ context.Context, in app.ImportRequest) (app.Imported, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	r := in.Record
	origin := r.OriginChamber
	if origin == "" {
		origin = r.Chamber
	}
	name := fmt.Sprintf("%s %d/%d", r.Kind, r.Number, r.Year)
	for _, p := range c.w.props {
		if p.Name == name && p.Origin == origin {
			return app.Imported{}, app.ErrPropositionExists
		}
	}
	for _, l := range c.w.links {
		if l.Confirmed && l.Chamber == r.Chamber && l.ExternalID == r.ExternalID {
			return app.Imported{}, app.ErrLinkTaken
		}
	}
	id := c.w.nextID("prop")
	c.w.props[id] = Proposition{ID: id, Name: name, Origin: origin, Record: r, Editor: in.EditorID}
	lid := c.w.nextID("link")
	c.w.links[lid] = Link{LinkRef: app.LinkRef{ID: lid, PropositionID: id, Chamber: r.Chamber, ExternalID: r.ExternalID, Confirmed: true}}
	suggested := 0
	for _, cand := range in.Candidates {
		taken := false
		for _, l := range c.w.links {
			if l.Confirmed && l.Chamber == cand.Chamber && l.ExternalID == cand.ExternalID {
				taken = true
			}
		}
		if taken || cand.Chamber == r.Chamber {
			continue
		}
		sid := c.w.nextID("link")
		// Sugestão: Confirmed fica false. Só o Editor confirma.
		c.w.links[sid] = Link{LinkRef: app.LinkRef{ID: sid, PropositionID: id, Chamber: cand.Chamber, ExternalID: cand.ExternalID}}
		suggested++
	}
	return app.Imported{PropositionID: id, Name: name, Suggested: suggested}, nil
}

func (c catalog) Link(_ context.Context, id string) (app.LinkRef, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	l, ok := c.w.links[id]
	if !ok {
		return app.LinkRef{}, app.ErrLinkNotFound
	}
	return l.LinkRef, nil
}

func (c catalog) ConfirmedLinkIDs(context.Context) ([]string, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	var out []string
	for _, l := range c.w.links {
		if l.Confirmed {
			out = append(out, l.ID)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (c catalog) ApplySync(_ context.Context, in app.SyncResult) error {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	l, ok := c.w.links[in.LinkID]
	if !ok || !l.Confirmed {
		return app.ErrLinkNotFound
	}
	p := c.w.props[l.PropositionID]
	p.Syncs++
	p.Updated = in.Record
	c.w.props[p.ID] = p
	l.Synced++
	l.LastError = ""
	c.w.links[l.ID] = l
	return nil
}

func (c catalog) RecordSyncFailure(_ context.Context, id, msg string) error {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	l, ok := c.w.links[id]
	if !ok {
		return app.ErrLinkNotFound
	}
	l.LastError = msg
	c.w.links[id] = l
	return nil
}

// Progress1 monta um evento de Tramitação de teste.
func Progress1(key string, at time.Time, desc string) app.SourceProgress {
	return app.SourceProgress{ExternalKey: key, At: at, Description: desc}
}

// ErrBoom é um erro qualquer para simular falha de infraestrutura.
var ErrBoom = errors.New("fakes: falha simulada")

// --- Radar ---

type radarKey struct {
	chamber app.ChamberCode
	id      string
}

// Radar implementa app.RadarRepo em memória, com as mesmas regras do
// banco (pauta mais recente fica; dispensado volta com pauta posterior).
type Radar struct {
	mu        sync.Mutex
	items     map[radarKey]app.RadarItem
	dismissAg map[radarKey]*time.Time
	lastSeen  map[radarKey]time.Time
}

func NewRadar() *Radar {
	return &Radar{items: map[radarKey]app.RadarItem{}, dismissAg: map[radarKey]*time.Time{}, lastSeen: map[radarKey]time.Time{}}
}

func (r *Radar) Upsert(_ context.Context, hits []app.RadarHit, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, h := range hits {
		k := radarKey{h.Chamber, h.ExternalID}
		it, ok := r.items[k]
		if !ok {
			it = app.RadarItem{RadarHit: h, FirstSeenAt: now}
		} else {
			if h.AgendaAt != nil && (it.AgendaAt == nil || h.AgendaAt.After(*it.AgendaAt)) {
				it.AgendaAt = h.AgendaAt
			}
			if it.PresentedAt == nil {
				it.PresentedAt = h.PresentedAt
			}
		}
		r.items[k] = it
		r.lastSeen[k] = now
	}
	return nil
}

func (r *Radar) Visible(_ context.Context, agendaSince, presentedSince time.Time) ([]app.RadarItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []app.RadarItem
	for k, it := range r.items {
		recent := (it.AgendaAt != nil && !it.AgendaAt.Before(agendaSince)) || (it.PresentedAt != nil && !it.PresentedAt.Before(presentedSince))
		if !recent {
			continue
		}
		if it.DismissedAt != nil {
			d := r.dismissAg[k]
			if it.AgendaAt == nil || (d != nil && !it.AgendaAt.After(*d)) {
				continue
			}
		}
		out = append(out, it)
	}
	return out, nil
}

func (r *Radar) Dismiss(_ context.Context, c app.ChamberCode, id, _ string, now time.Time) (app.RadarItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := radarKey{c, id}
	it, ok := r.items[k]
	if !ok {
		return app.RadarItem{}, app.ErrRadarItemNotFound
	}
	it.DismissedAt = &now
	r.items[k] = it
	r.dismissAg[k] = it.AgendaAt
	return it, nil
}

func (r *Radar) Purge(_ context.Context, before time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for k, t := range r.lastSeen {
		if t.Before(before) {
			delete(r.items, k)
			delete(r.lastSeen, k)
			n++
		}
	}
	return n, nil
}

// Len é quantos itens o Radar guarda.
func (r *Radar) Len() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.items) }

func (r *Radar) LastSeen(context.Context) (*time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var last *time.Time
	for _, t := range r.lastSeen {
		if last == nil || t.After(*last) {
			t := t
			last = &t
		}
	}
	return last, nil
}

// RadarScheduler conta os pedidos de rodada.
type RadarScheduler struct{ Calls int }

func (s *RadarScheduler) ScheduleRadarScan(context.Context) error { s.Calls++; return nil }
