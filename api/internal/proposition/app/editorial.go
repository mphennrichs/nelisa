package app

import (
	"context"
	"errors"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// Editorial são as operações de sistema sobre Versões que a revisão
// (módulo review) e a Fila (módulo queue) usam, ligadas no main. Como o
// Catalog: não autorizam, não auditam e não abrem transação; quem
// orquestra faz as três coisas.
type Editorial struct {
	repos EditorialRepos
	clock clock.Clock
}

func NewEditorial(repos EditorialRepos, clk clock.Clock) *Editorial {
	return &Editorial{repos: repos, clock: clk}
}

// ApprovalSubject é a Versão em análise como a revisão a enxerga: estado,
// Autoria e o que falta no conteúdo. Nunca o texto.
type ApprovalSubject struct {
	PropositionID string
	Name          string
	ReviewState   domain.ReviewState
	Version       domain.Version
	Revision      int
	Authors       []string
	Missing       []domain.Missing
}

func (e *Editorial) subject(ctx context.Context, p domain.Proposition, v domain.Version) (ApprovalSubject, error) {
	c, err := e.repos.Contents.FindByVersion(ctx, v.ID)
	if err != nil {
		return ApprovalSubject{}, err
	}
	topics, err := e.repos.Topics.List(ctx)
	if err != nil {
		return ApprovalSubject{}, err
	}
	missing := c.MissingForApproval(domain.TopicsByID(topics))
	st, err := e.repos.Structurings.FindByVersion(ctx, v.ID)
	switch {
	case err == nil:
		missing = append(missing, st.MissingForApproval()...)
	case !errors.Is(err, domain.ErrStructuringNotFound):
		return ApprovalSubject{}, err
	}
	return ApprovalSubject{
		PropositionID: p.ID, Name: p.Name(), ReviewState: p.ReviewState, Version: v, Revision: c.Revision,
		Authors: append([]string(nil), c.Authors...), Missing: missing,
	}, nil
}

// PendingSubject devolve a Versão em análise da Proposição, ou nil se não
// há.
func (e *Editorial) PendingSubject(ctx context.Context, propositionID string) (*ApprovalSubject, error) {
	p, err := e.repos.Props.FindByID(ctx, propositionID)
	if err != nil {
		return nil, err
	}
	vs, err := e.repos.Versions.ListByProposition(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	_, pending := domain.FindVersions(vs)
	if pending == nil {
		return nil, nil
	}
	s, err := e.subject(ctx, p, *pending)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// LockForApproval trava a Proposição até o fim da transação do contexto e
// devolve a Versão pedida, que tem de ser dela e estar em análise.
func (e *Editorial) LockForApproval(ctx context.Context, propositionID, versionID string) (ApprovalSubject, error) {
	p, err := e.repos.Props.FindByIDForUpdate(ctx, propositionID)
	if err != nil {
		return ApprovalSubject{}, err
	}
	v, err := e.repos.Versions.FindByID(ctx, versionID)
	if err != nil {
		return ApprovalSubject{}, err
	}
	if v.PropositionID != p.ID {
		return ApprovalSubject{}, domain.ErrVersionNotFound
	}
	if v.State != domain.VersionPending {
		return ApprovalSubject{}, domain.ErrVersionNotPending
	}
	return e.subject(ctx, p, v)
}

// Promote aplica a aprovação: a Versão em análise vira a vigente, a
// vigente anterior vira substituída e a Proposição fica pronta. Chame
// dentro da transação de LockForApproval.
func (e *Editorial) Promote(ctx context.Context, propositionID, versionID string) error {
	p, err := e.repos.Props.FindByIDForUpdate(ctx, propositionID)
	if err != nil {
		return err
	}
	vs, err := e.repos.Versions.ListByProposition(ctx, p.ID)
	if err != nil {
		return err
	}
	current, pending := domain.FindVersions(vs)
	if pending == nil || pending.ID != versionID {
		return domain.ErrVersionNotPending
	}
	now := e.clock.Now()
	if err := p.MarkReady(now); err != nil {
		return err
	}
	// A ordem importa: o índice de uma vigente por Proposição vale a cada
	// comando, então a anterior sai antes de a nova entrar.
	if current != nil {
		if err := e.repos.Versions.UpdateState(ctx, current.ID, domain.VersionCurrent, domain.VersionSuperseded); err != nil {
			return err
		}
	}
	if err := e.repos.Versions.UpdateState(ctx, versionID, domain.VersionPending, domain.VersionCurrent); err != nil {
		return err
	}
	return e.repos.Props.Update(ctx, p)
}

// VersionAuthors devolve a Versão e a Autoria do conteúdo dela (segunda
// revisão).
func (e *Editorial) VersionAuthors(ctx context.Context, propositionID, versionID string) (domain.Version, []string, error) {
	v, err := e.repos.Versions.FindByID(ctx, versionID)
	if err != nil {
		return domain.Version{}, nil, err
	}
	if v.PropositionID != propositionID {
		return domain.Version{}, nil, domain.ErrVersionNotFound
	}
	c, err := e.repos.Contents.FindByVersion(ctx, v.ID)
	if err != nil {
		return domain.Version{}, nil, err
	}
	return v, append([]string(nil), c.Authors...), nil
}

// PropositionBrief é o que a Fila mostra de uma Proposição.
type PropositionBrief struct {
	ID          string
	Name        string
	Slug        string
	Sphere      domain.Sphere
	ReviewState domain.ReviewState
	Current     *domain.Version
	Pending     *domain.Version
	// PendingMissing é o que falta para aprovar a Versão em análise.
	PendingMissing []domain.Missing
}

// Exists diz se a Proposição existe.
func (e *Editorial) Exists(ctx context.Context, id string) (bool, error) {
	_, err := e.repos.Props.FindByID(ctx, id)
	if errors.Is(err, domain.ErrPropositionNotFound) {
		return false, nil
	}
	return err == nil, err
}

// InReview lista as Proposições "em revisão" com Versão em análise: os
// itens de revisão da Fila.
func (e *Editorial) InReview(ctx context.Context) ([]PropositionBrief, error) {
	ps, err := e.repos.Props.ListInStates(ctx, []domain.ReviewState{domain.ReviewInReview})
	if err != nil {
		return nil, err
	}
	out, err := e.briefs(ctx, ps)
	if err != nil {
		return nil, err
	}
	kept := out[:0]
	for _, b := range out {
		if b.Pending != nil {
			kept = append(kept, b)
		}
	}
	return kept, nil
}

// Describe devolve as Proposições pedidas (as que não existem ficam de
// fora).
func (e *Editorial) Describe(ctx context.Context, ids []string) ([]PropositionBrief, error) {
	var ps []domain.Proposition
	for _, id := range ids {
		p, err := e.repos.Props.FindByID(ctx, id)
		if errors.Is(err, domain.ErrPropositionNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	return e.briefs(ctx, ps)
}

func (e *Editorial) briefs(ctx context.Context, ps []domain.Proposition) ([]PropositionBrief, error) {
	if len(ps) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	vs, err := e.repos.Versions.ListByPropositions(ctx, ids)
	if err != nil {
		return nil, err
	}
	byProp := map[string][]domain.Version{}
	var pendingIDs []string
	for _, v := range vs {
		byProp[v.PropositionID] = append(byProp[v.PropositionID], v)
		if v.State == domain.VersionPending {
			pendingIDs = append(pendingIDs, v.ID)
		}
	}
	contents := map[string]domain.Content{}
	if len(pendingIDs) > 0 {
		cs, err := e.repos.Contents.ListByVersions(ctx, pendingIDs)
		if err != nil {
			return nil, err
		}
		for _, c := range cs {
			contents[c.VersionID] = c
		}
	}
	topics, err := e.repos.Topics.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := domain.TopicsByID(topics)
	out := make([]PropositionBrief, 0, len(ps))
	for _, p := range ps {
		b := PropositionBrief{ID: p.ID, Name: p.Name(), Slug: p.Slug, Sphere: p.Sphere, ReviewState: p.ReviewState}
		b.Current, b.Pending = domain.FindVersions(byProp[p.ID])
		if b.Pending != nil {
			b.PendingMissing = contents[b.Pending.ID].MissingForApproval(byID)
		}
		out = append(out, b)
	}
	return out, nil
}
