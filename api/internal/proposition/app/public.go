package app

import (
	"context"
	"errors"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// Camada pública: todo dado que sai por /api/public passa por aqui e pela
// regra de visibilidade do domínio (domain.PublicView). Sem Ator: o Leitor
// é anônimo. O que não é público responde ErrPropositionNotFound, sem
// revelar que existe.

// PublicTimelineEntry é um evento de Tramitação como o Leitor o vê.
type PublicTimelineEntry = TimelineEntry

// PublicPropositionDetail é a página pública de uma Proposição.
type PublicPropositionDetail struct {
	domain.PublicProposition
	Timeline []PublicTimelineEntry
}

// ListPublicPropositions lista o que o Leitor pode ver.
type ListPublicPropositions struct {
	props PropositionRepo
	links LinkRepo
}

func NewListPublicPropositions(props PropositionRepo, links LinkRepo) *ListPublicPropositions {
	return &ListPublicPropositions{props: props, links: links}
}

func (uc *ListPublicPropositions) Execute(ctx context.Context) ([]domain.PublicProposition, error) {
	// O filtro no banco é só eficiência; quem decide é PublicView abaixo.
	ps, err := uc.props.ListInStates(ctx, domain.PublicStates())
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
	out := make([]domain.PublicProposition, 0, len(ps))
	for _, p := range ps {
		// Na lista, a Apensação vai só pelo nome; a página traz o resto.
		if v, ok := domain.PublicView(p, byProp[p.ID], nil, nil); ok {
			out = append(out, v)
		}
	}
	return out, nil
}

// GetPublicProposition é a página pública de uma Proposição, pelo slug.
type GetPublicProposition struct {
	props    PropositionRepo
	links    LinkRepo
	timeline TimelineReader
}

func NewGetPublicProposition(props PropositionRepo, links LinkRepo, timeline TimelineReader) *GetPublicProposition {
	return &GetPublicProposition{props: props, links: links, timeline: timeline}
}

func (uc *GetPublicProposition) Execute(ctx context.Context, slug string) (PublicPropositionDetail, error) {
	p, err := uc.props.FindBySlug(ctx, slug)
	if err != nil {
		return PublicPropositionDetail{}, err
	}
	if !p.ReviewState.IsPublic() {
		// Antes de ler qualquer outra coisa: rascunho é 404.
		return PublicPropositionDetail{}, domain.ErrPropositionNotFound
	}
	links, err := uc.links.ListByProposition(ctx, p.ID)
	if err != nil {
		return PublicPropositionDetail{}, err
	}
	principal, attached, err := relatives(ctx, uc.props, p)
	if err != nil && !errors.Is(err, domain.ErrPropositionNotFound) {
		return PublicPropositionDetail{}, err
	}
	view, ok := domain.PublicView(p, links, principal, attached)
	if !ok {
		return PublicPropositionDetail{}, domain.ErrPropositionNotFound
	}
	tl, err := uc.timeline.Timeline(ctx, p.ID)
	if err != nil {
		return PublicPropositionDetail{}, err
	}
	return PublicPropositionDetail{PublicProposition: view, Timeline: tl}, nil
}
