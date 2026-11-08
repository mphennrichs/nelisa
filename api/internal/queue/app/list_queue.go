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
}

// ListQueue monta a Fila: Proposições "em revisão" com Versão em análise
// e Proposições prontas com Segunda revisão pendente, sempre por Esfera.
type ListQueue struct {
	props   Propositions
	reviews SecondReviews
	claims  ClaimRepo
	people  People
	clock   clock.Clock
}

func NewListQueue(props Propositions, reviews SecondReviews, claims ClaimRepo, people People, clk clock.Clock) *ListQueue {
	return &ListQueue{props: props, reviews: reviews, claims: claims, people: people, clock: clk}
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
	if len(pending) > 0 {
		ids := make([]string, 0, len(pending))
		for _, s := range pending {
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
		for _, s := range pending {
			p, ok := byID[s.PropositionID]
			// Só a aprovação da Versão vigente de uma Proposição pronta:
			// de uma Versão já substituída o Leitor não vê mais nada.
			if !ok || !p.Ready || p.Current == nil || p.Current.ID != s.VersionID || !keep(p) {
				continue
			}
			items = append(items, QueueItem{
				Item: domain.Item{PropositionID: p.ID, Sphere: p.Sphere, Reason: domain.ReasonSecondReview, Since: s.ApprovedAt},
				Name: p.Name, Slug: p.Slug, Version: *p.Current, Missing: []Missing{},
			})
		}
	}
	if err := uc.attachClaims(ctx, items, uc.clock.Now()); err != nil {
		return nil, err
	}
	domain.Sort(items, func(i QueueItem) domain.Item { return i.Item })
	return items, nil
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
