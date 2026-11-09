package fakes

import (
	"context"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

func (w *World) Versions() app.VersionRepo { return versions{w} }
func (w *World) Contents() app.ContentRepo { return contents{w} }
func (w *World) Topics() app.TopicRepo     { return topics{w} }
func (w *World) Claims() app.Claims        { return claims{w} }
func (w *World) Reviews() Reviews          { return reviews{w} }
func (w *World) People() app.People        { return people{w} }

// Reviews é o que o módulo review oferece ao proposition.
type Reviews interface {
	app.Reviews
	app.PublicReviews
}

// Repos são os repositórios do ciclo editorial.
func (w *World) Repos() app.EditorialRepos {
	return app.EditorialRepos{
		Props: w.Props(), Versions: w.Versions(), Contents: w.Contents(), Topics: w.Topics(),
		Structurings: w.Structurings(), Comments: w.Comments(),
	}
}

// PutVersion, PutContent e PutTopic gravam direto (montagem de teste).
func (w *World) PutVersion(v domain.Version) { w.mu.Lock(); defer w.mu.Unlock(); w.versions[v.ID] = v }
func (w *World) PutContent(c domain.Content) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.contents[c.VersionID] = c
}
func (w *World) PutTopic(t domain.Topic) { w.mu.Lock(); defer w.mu.Unlock(); w.topics[t.ID] = t }

// SetClaim põe a Posse com alguém (nil solta).
func (w *World) SetClaim(propositionID string, c *app.ClaimInfo) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if c == nil {
		delete(w.claims, propositionID)
		return
	}
	w.claims[propositionID] = *c
}

// Claim devolve a Posse guardada.
func (w *World) Claim(propositionID string) (app.ClaimInfo, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.claims[propositionID]
	return c, ok
}

// VersionsOf devolve as Versões de uma Proposição, por seq.
func (w *World) VersionsOf(propositionID string) []domain.Version {
	vs, _ := versions{w}.ListByProposition(context.Background(), propositionID)
	return vs
}

// ContentOf devolve o conteúdo de uma Versão.
func (w *World) ContentOf(versionID string) (domain.Content, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.contents[versionID]
	return c, ok
}

// --- Versões ---

type versions struct{ w *World }

func (r versions) Create(_ context.Context, v domain.Version) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	for _, o := range r.w.versions {
		if o.PropositionID == v.PropositionID && (o.Seq == v.Seq || (o.State == domain.VersionPending && v.State == domain.VersionPending)) {
			return domain.ErrPendingVersionExists
		}
	}
	r.w.versions[v.ID] = v
	return nil
}

func (r versions) FindByID(_ context.Context, id string) (domain.Version, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	v, ok := r.w.versions[id]
	if !ok {
		return domain.Version{}, domain.ErrVersionNotFound
	}
	return v, nil
}

func (r versions) filter(keep func(domain.Version) bool) []domain.Version {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.Version
	for _, v := range r.w.versions {
		if keep(v) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PropositionID != out[j].PropositionID {
			return out[i].PropositionID < out[j].PropositionID
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}

func (r versions) ListByProposition(_ context.Context, id string) ([]domain.Version, error) {
	return r.filter(func(v domain.Version) bool { return v.PropositionID == id }), nil
}

func (r versions) ListByPropositions(_ context.Context, ids []string) ([]domain.Version, error) {
	return r.filter(func(v domain.Version) bool { return slices.Contains(ids, v.PropositionID) }), nil
}

func (r versions) UpdateState(_ context.Context, id string, from, to domain.VersionState) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	v, ok := r.w.versions[id]
	if !ok || v.State != from {
		return domain.ErrVersionNotPending
	}
	for _, o := range r.w.versions {
		if o.ID != id && o.PropositionID == v.PropositionID && o.State == to && to != domain.VersionSuperseded {
			return domain.ErrVersionNotPending // imita o índice único
		}
	}
	v.State = to
	r.w.versions[id] = v
	return nil
}

// --- Conteúdo ---

type contents struct{ w *World }

func (r contents) Create(_ context.Context, c domain.Content) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.contents[c.VersionID] = c
	return nil
}

func (r contents) FindByVersion(_ context.Context, id string) (domain.Content, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	c, ok := r.w.contents[id]
	if !ok {
		return domain.Content{}, domain.ErrVersionNotFound
	}
	return c, nil
}

func (r contents) ListByVersions(_ context.Context, ids []string) ([]domain.Content, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.Content
	for _, id := range ids {
		if c, ok := r.w.contents[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r contents) Update(_ context.Context, c domain.Content, prev int) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	cur, ok := r.w.contents[c.VersionID]
	if !ok || cur.Revision != prev {
		return domain.ErrStaleContent
	}
	r.w.contents[c.VersionID] = c
	return nil
}

// --- Temas ---

type topics struct{ w *World }

func (r topics) nameTaken(t domain.Topic) bool {
	for _, o := range r.w.topics {
		if o.ID != t.ID && strings.EqualFold(o.Name, t.Name) {
			return true
		}
	}
	return false
}

func (r topics) Create(_ context.Context, t domain.Topic) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if r.nameTaken(t) {
		return domain.ErrTopicNameTaken
	}
	r.w.topics[t.ID] = t
	return nil
}

func (r topics) FindByID(_ context.Context, id string) (domain.Topic, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	t, ok := r.w.topics[id]
	if !ok {
		return domain.Topic{}, domain.ErrTopicNotFound
	}
	return t, nil
}

func (r topics) List(context.Context) ([]domain.Topic, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	out := slices.Collect(maps.Values(r.w.topics))
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

func (r topics) Update(_ context.Context, t domain.Topic) error {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if _, ok := r.w.topics[t.ID]; !ok {
		return domain.ErrTopicNotFound
	}
	if r.nameTaken(t) {
		return domain.ErrTopicNameTaken
	}
	r.w.topics[t.ID] = t
	return nil
}

// --- Posse (o que o módulo queue faria) ---

type claims struct{ w *World }

// TouchForEdit imita a regra do queue: livre assume, própria renova,
// alheia recusa. A expiração e a conta inativa são testadas no queue.
func (c claims) TouchForEdit(_ context.Context, propositionID, userID string) error {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	cur, ok := c.w.claims[propositionID]
	if ok && cur.UserID != userID {
		return &app.ClaimTakenError{HolderName: cur.HolderName}
	}
	now := time.Now().UTC()
	if !ok {
		cur = app.ClaimInfo{UserID: userID, HolderName: c.w.Names[userID], ClaimedAt: now}
	}
	cur.LastActivityAt, cur.ExpiresAt = now, now.Add(72*time.Hour)
	c.w.claims[propositionID] = cur
	return nil
}

func (c claims) Current(_ context.Context, propositionID string) (*app.ClaimInfo, error) {
	c.w.mu.Lock()
	defer c.w.mu.Unlock()
	cur, ok := c.w.claims[propositionID]
	if !ok {
		return nil, nil
	}
	return &cur, nil
}

// --- Revisão (o que o módulo review faria) ---

type reviews struct{ w *World }

func (r reviews) Status(_ context.Context, propositionID string) (app.ReviewStatus, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	return r.w.Statuses[propositionID], nil
}

func (r reviews) Badges(_ context.Context, ids []string) (map[string]int, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	out := map[string]int{}
	for _, id := range ids {
		if b, ok := r.w.Badges[id]; ok {
			out[id] = b
		}
	}
	return out, nil
}

func (r reviews) ApprovedErrata(_ context.Context, ids []string) ([]domain.PublicErratum, error) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	var out []domain.PublicErratum
	for _, e := range r.w.Errata {
		if slices.Contains(ids, e.VersionID) {
			out = append(out, e)
		}
	}
	return out, nil
}

// --- Nomes ---

type people struct{ w *World }

func (p people) Names(_ context.Context, ids []string) (map[string]string, error) {
	p.w.mu.Lock()
	defer p.w.mu.Unlock()
	out := map[string]string{}
	for _, id := range ids {
		if n, ok := p.w.Names[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}
