package app

import (
	"context"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
)

type ListQueueInput struct {
	// Sphere filtra por Esfera ("" = todas).
	Sphere string
}

// QueueItem é uma entrada da Fila de revisão, para a equipe.
type QueueItem struct {
	domain.Item
	Name    string
	Slug    string
	Version Version
	// Claim é a Posse que vale (nil se livre).
	Claim *ClaimView
	// Missing é o que falta para aprovar (só nos itens de revisão).
	Missing []Missing
	// Contribution: só nos itens de Contribuição (Version fica vazia).
	Contribution *ContributionItem
}

// ContributionItem é o que a Fila mostra de uma Contribuição pendente.
type ContributionItem struct {
	ID      string
	Kind    string
	Subject string
}

// ListQueue monta a Fila, sempre por Esfera: Proposições "em revisão" com
// Versão em análise, Proposições prontas com Segunda revisão pendente, com
// Correção menor a conferir (um item por Proposição, desde a mais antiga)
// e com Errata pendente; e as Contribuições pendentes (desenho da fase 3,
// "Moderação pela Fila"), na mesma ordem.
type ListQueue struct {
	props    Propositions
	reviews  ReviewWork
	contribs Contributions
	claims   ClaimRepo
	people   People
	clock    clock.Clock
}

func NewListQueue(props Propositions, reviews ReviewWork, contribs Contributions, claims ClaimRepo, people People, clk clock.Clock) *ListQueue {
	return &ListQueue{props: props, reviews: reviews, contribs: contribs, claims: claims, people: people, clock: clk}
}

func (uc *ListQueue) Execute(ctx context.Context, in ListQueueInput) ([]QueueItem, error) {
	if _, err := actor(ctx); err != nil {
		return nil, err
	}
	var sphere domain.Sphere
	if in.Sphere != "" {
		s, err := domain.ParseSphere(in.Sphere)
		if err != nil {
			return nil, err
		}
		sphere = s
	}
	keep := func(p Proposition) bool { return sphere == "" || p.Sphere == sphere }

	var items []QueueItem
	inReview, err := uc.props.InReview(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range inReview {
		if p.Pending == nil || !keep(p) {
			continue
		}
		items = append(items, QueueItem{
			Item: domain.Item{PropositionID: p.ID, Sphere: p.Sphere, Reason: domain.ReasonReview, Since: p.Pending.CreatedAt},
			Name: p.Name, Slug: p.Slug, Version: *p.Pending, Missing: p.Missing,
		})
	}

	pending, err := uc.reviews.PendingSecondReviews(ctx)
	if err != nil {
		return nil, err
	}
	work := make([]PendingCorrection, 0, len(pending))
	for _, s := range pending {
		work = append(work, PendingCorrection{PropositionID: s.PropositionID, VersionID: s.VersionID, Reason: domain.ReasonSecondReview, Since: s.ApprovedAt})
	}
	corrections, err := uc.reviews.PendingCorrections(ctx)
	if err != nil {
		return nil, err
	}
	work = append(work, corrections...)
	if len(work) > 0 {
		ids := make([]string, 0, len(work))
		for _, s := range work {
			ids = append(ids, s.PropositionID)
		}
		described, err := uc.props.Describe(ctx, ids)
		if err != nil {
			return nil, err
		}
		byID := map[string]Proposition{}
		for _, p := range described {
			byID[p.ID] = p
		}
		// Um item por Proposição e motivo: três Correções a conferir na
		// mesma Proposição são um trabalho só, desde a mais antiga.
		seen := map[string]int{}
		for _, s := range work {
			p, ok := byID[s.PropositionID]
			// Só o que é da Versão vigente de uma Proposição pronta: de uma
			// Versão já substituída o Leitor não vê mais nada.
			if !ok || !p.Ready || p.Current == nil || p.Current.ID != s.VersionID || !keep(p) {
				continue
			}
			key := p.ID + "/" + string(s.Reason)
			if i, dup := seen[key]; dup {
				if s.Since.Before(items[i].Since) {
					items[i].Since = s.Since
				}
				continue
			}
			seen[key] = len(items)
			items = append(items, QueueItem{
				Item: domain.Item{PropositionID: p.ID, Sphere: p.Sphere, Reason: s.Reason, Since: s.Since},
				Name: p.Name, Slug: p.Slug, Version: *p.Current, Missing: []Missing{},
			})
		}
	}
	if err := uc.attachClaims(ctx, items, uc.clock.Now()); err != nil {
		return nil, err
	}
	contribs, err := uc.contributionItems(ctx, sphere)
	if err != nil {
		return nil, err
	}
	items = append(items, contribs...)
	domain.Sort(items, func(i QueueItem) domain.Item { return i.Item })
	return items, nil
}

// contributionItems monta os itens das Contribuições pendentes. A Posse
// não vale para elas: moderar não edita a Proposição.
func (uc *ListQueue) contributionItems(ctx context.Context, sphere domain.Sphere) ([]QueueItem, error) {
	cs, err := uc.contribs.Pending(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, c := range cs {
		if c.PropositionID != "" {
			ids = append(ids, c.PropositionID)
		}
	}
	byID := map[string]Proposition{}
	if len(ids) > 0 {
		described, err := uc.props.Describe(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, p := range described {
			byID[p.ID] = p
		}
	}
	var out []QueueItem
	for _, c := range cs {
		if sphere != "" && c.Sphere != sphere {
			continue
		}
		it := QueueItem{
			Item: domain.Item{
				PropositionID: c.PropositionID, ContributionID: c.ID, Sphere: c.Sphere,
				Reason: domain.ReasonContribution, Since: c.SubmittedAt,
			},
			Name: c.Subject, Missing: []Missing{},
			Contribution: &ContributionItem{ID: c.ID, Kind: c.Kind, Subject: c.Subject},
		}
		if p, ok := byID[c.PropositionID]; ok {
			it.Name, it.Slug = p.Name, p.Slug
		}
		out = append(out, it)
	}
	return out, nil
}

// attachClaims põe em cada item a Posse que vale, com o nome de quem a
// tem. Posse expirada ou de conta desativada aparece como livre.
func (uc *ListQueue) attachClaims(ctx context.Context, items []QueueItem, now time.Time) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.PropositionID)
	}
	cs, err := uc.claims.ListByPropositions(ctx, ids)
	if err != nil {
		return err
	}
	byProp := map[string]domain.Claim{}
	var users []string
	for _, c := range cs {
		byProp[c.PropositionID] = c
		users = append(users, c.UserID)
	}
	if len(users) == 0 {
		return nil
	}
	people, err := uc.people.Lookup(ctx, users)
	if err != nil {
		return err
	}
	for i := range items {
		c, ok := byProp[items[i].PropositionID]
		if !ok {
			continue
		}
		p := people[c.UserID]
		if c.Valid(now, p.Active) {
			items[i].Claim = &ClaimView{Claim: c, HolderName: p.Name}
		}
	}
	return nil
}
