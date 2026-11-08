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

// publicEditorial junta o ciclo editorial de várias Proposições para a
// regra de visibilidade. Só lê o conteúdo das Versões vigentes: o da Versão
// em análise nem sai do banco. A regra (domain.PublicView) confere de novo.
func publicEditorial(ctx context.Context, repos EditorialRepos, badges BadgeReader, propositionIDs []string) (domain.Editorial, error) {
	ed := domain.Editorial{}
	if len(propositionIDs) == 0 {
		return ed, nil
	}
	vs, err := repos.Versions.ListByPropositions(ctx, propositionIDs)
	if err != nil {
		return ed, err
	}
	ed.Versions = vs
	var currentIDs []string
	for _, v := range vs {
		if v.State == domain.VersionCurrent {
			currentIDs = append(currentIDs, v.ID)
		}
	}
	if len(currentIDs) == 0 {
		return ed, nil
	}
	if ed.Contents, err = repos.Contents.ListByVersions(ctx, currentIDs); err != nil {
		return ed, err
	}
	if ed.Badges, err = badges.Badges(ctx, currentIDs); err != nil {
		return ed, err
	}
	topics, err := repos.Topics.List(ctx)
	if err != nil {
		return ed, err
	}
	ed.Topics = domain.TopicsByID(topics)
	return ed, nil
}

// ListPublicPropositions lista o que o Leitor pode ver.
type ListPublicPropositions struct {
	repos  EditorialRepos
	links  LinkRepo
	badges BadgeReader
}

func NewListPublicPropositions(repos EditorialRepos, links LinkRepo, badges BadgeReader) *ListPublicPropositions {
	return &ListPublicPropositions{repos: repos, links: links, badges: badges}
}

func (uc *ListPublicPropositions) Execute(ctx context.Context) ([]domain.PublicProposition, error) {
	// O filtro no banco é só eficiência; quem decide é PublicView abaixo.
	ps, err := uc.repos.Props.ListInStates(ctx, domain.PublicStates())
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
	ed, err := publicEditorial(ctx, uc.repos, uc.badges, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PublicProposition, 0, len(ps))
	for _, p := range ps {
		// Na lista, a Apensação vai só pelo nome; a página traz o resto.
		if v, ok := domain.PublicView(p, byProp[p.ID], nil, nil, ed); ok {
			out = append(out, v)
		}
	}
	return out, nil
}

// GetPublicProposition é a página pública de uma Proposição, pelo slug.
type GetPublicProposition struct {
	repos    EditorialRepos
	links    LinkRepo
	timeline TimelineReader
	badges   BadgeReader
}

func NewGetPublicProposition(repos EditorialRepos, links LinkRepo, timeline TimelineReader, badges BadgeReader) *GetPublicProposition {
	return &GetPublicProposition{repos: repos, links: links, timeline: timeline, badges: badges}
}

func (uc *GetPublicProposition) Execute(ctx context.Context, slug string) (PublicPropositionDetail, error) {
	p, err := uc.repos.Props.FindBySlug(ctx, slug)
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
	principal, attached, err := relatives(ctx, uc.repos.Props, p)
	if err != nil && !errors.Is(err, domain.ErrPropositionNotFound) {
		return PublicPropositionDetail{}, err
	}
	ed, err := publicEditorial(ctx, uc.repos, uc.badges, []string{p.ID})
	if err != nil {
		return PublicPropositionDetail{}, err
	}
	view, ok := domain.PublicView(p, links, principal, attached, ed)
	if !ok {
		return PublicPropositionDetail{}, domain.ErrPropositionNotFound
	}
	tl, err := uc.timeline.Timeline(ctx, p.ID)
	if err != nil {
		return PublicPropositionDetail{}, err
	}
	return PublicPropositionDetail{PublicProposition: view, Timeline: tl}, nil
}
