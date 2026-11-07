package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// TeamProposition é a Proposição como a equipe a vê: tudo, inclusive
// estado de Revisão, sugestões e erros de Sincronização.
type TeamProposition struct {
	domain.Proposition
	Links []domain.SourceLink
}

// TeamPropositionDetail acrescenta a Tramitação e a Apensação.
type TeamPropositionDetail struct {
	TeamProposition
	Principal *domain.Proposition
	Attached  []domain.Proposition
	Timeline  []TimelineEntry
}

type ListPropositionsInput struct{}

// ListPropositions lista todas as Proposições para a equipe.
type ListPropositions struct {
	props PropositionRepo
	links LinkRepo
}

func NewListPropositions(props PropositionRepo, links LinkRepo) *ListPropositions {
	return &ListPropositions{props: props, links: links}
}

func (uc *ListPropositions) Execute(ctx context.Context, _ ListPropositionsInput) ([]TeamProposition, error) {
	if _, err := requireEditor(ctx); err != nil {
		return nil, err
	}
	ps, err := uc.props.List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	byProp := map[string][]domain.SourceLink{}
	if len(ids) > 0 {
		links, err := uc.links.ListByPropositions(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, l := range links {
			byProp[l.PropositionID] = append(byProp[l.PropositionID], l)
		}
	}
	out := make([]TeamProposition, 0, len(ps))
	for _, p := range ps {
		out = append(out, TeamProposition{Proposition: p, Links: byProp[p.ID]})
	}
	return out, nil
}

type GetPropositionInput struct {
	PropositionID string
}

// GetProposition mostra uma Proposição à equipe, com Vínculos e Tramitação.
type GetProposition struct {
	props    PropositionRepo
	links    LinkRepo
	timeline TimelineReader
}

func NewGetProposition(props PropositionRepo, links LinkRepo, timeline TimelineReader) *GetProposition {
	return &GetProposition{props: props, links: links, timeline: timeline}
}

func (uc *GetProposition) Execute(ctx context.Context, in GetPropositionInput) (TeamPropositionDetail, error) {
	if _, err := requireEditor(ctx); err != nil {
		return TeamPropositionDetail{}, err
	}
	p, err := uc.props.FindByID(ctx, in.PropositionID)
	if err != nil {
		return TeamPropositionDetail{}, err
	}
	links, err := uc.links.ListByProposition(ctx, p.ID)
	if err != nil {
		return TeamPropositionDetail{}, err
	}
	principal, attached, err := relatives(ctx, uc.props, p)
	if err != nil {
		return TeamPropositionDetail{}, err
	}
	tl, err := uc.timeline.Timeline(ctx, p.ID)
	if err != nil {
		return TeamPropositionDetail{}, err
	}
	return TeamPropositionDetail{
		TeamProposition: TeamProposition{Proposition: p, Links: links},
		Principal:       principal, Attached: attached, Timeline: tl,
	}, nil
}

// relatives devolve a principal (se resolvida) e as apensadas a p.
func relatives(ctx context.Context, props PropositionRepo, p domain.Proposition) (*domain.Proposition, []domain.Proposition, error) {
	var principal *domain.Proposition
	if p.AttachedToID != nil {
		pr, err := props.FindByID(ctx, *p.AttachedToID)
		if err != nil {
			return nil, nil, err
		}
		principal = &pr
	}
	attached, err := props.ListAttachedTo(ctx, p.ID)
	if err != nil {
		return nil, nil, err
	}
	return principal, attached, nil
}
