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
	Editorial TeamEditorial
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

// TeamEditorial é o ciclo editorial como a equipe o vê, com as decisões
// de permissão já tomadas pela API (o front não decide).
type TeamEditorial struct {
	Versions []domain.Version
	// Contents traz o conteúdo da Versão em análise e o da vigente.
	Contents map[string]domain.Content
	// Topics são todos os Temas, para mostrar nomes (inclusive de
	// desativados ainda em uso).
	Topics map[string]domain.Topic
	Claim  *ClaimInfo
	Review ReviewStatus
	// People são os nomes de quem aparece (Autoria, Posse, aprovação).
	People           map[string]string
	CanEdit          bool
	CanCreateVersion bool
	// CanMinorFix: o review permite (pronta, sem Errata pendente) e a
	// Posse está livre ou é do Ator.
	CanMinorFix bool
	// Structurings traz a Estruturação (com os Dispositivos) da Versão em
	// análise e da vigente.
	Structurings map[string]StructuringView
	// CanEditStructuring: corrigir/colar o texto e confirmar, na Versão
	// em análise, com a Posse livre ou do Ator.
	CanEditStructuring bool
	// CanRetryStructuring: a Estruturação da Versão em análise é manual
	// por falha de busca que vale tentar de novo.
	CanRetryStructuring bool
	// Comments traz os Comentários vivos da Versão em análise e da
	// vigente, por Versão.
	Comments map[string][]domain.Comment
	// CanComment: Estruturação da Versão em análise confirmada e Posse
	// livre ou do Ator.
	CanComment bool
}

// GetProposition mostra uma Proposição à equipe, com Vínculos, Tramitação,
// Versões, conteúdo editorial, Posse e Revisão.
type GetProposition struct {
	repos    EditorialRepos
	links    LinkRepo
	timeline TimelineReader
	claims   Claims
	reviews  Reviews
	people   People
}

func NewGetProposition(repos EditorialRepos, links LinkRepo, timeline TimelineReader, claims Claims, reviews Reviews, people People) *GetProposition {
	return &GetProposition{repos: repos, links: links, timeline: timeline, claims: claims, reviews: reviews, people: people}
}

func (uc *GetProposition) Execute(ctx context.Context, in GetPropositionInput) (TeamPropositionDetail, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return TeamPropositionDetail{}, err
	}
	p, err := uc.repos.Props.FindByID(ctx, in.PropositionID)
	if err != nil {
		return TeamPropositionDetail{}, err
	}
	links, err := uc.links.ListByProposition(ctx, p.ID)
	if err != nil {
		return TeamPropositionDetail{}, err
	}
	principal, attached, err := relatives(ctx, uc.repos.Props, p)
	if err != nil {
		return TeamPropositionDetail{}, err
	}
	tl, err := uc.timeline.Timeline(ctx, p.ID)
	if err != nil {
		return TeamPropositionDetail{}, err
	}
	ed, err := uc.editorial(ctx, p.ID, actor.UserID)
	if err != nil {
		return TeamPropositionDetail{}, err
	}
	return TeamPropositionDetail{
		TeamProposition: TeamProposition{Proposition: p, Links: links},
		Principal:       principal, Attached: attached, Timeline: tl, Editorial: ed,
	}, nil
}

func (uc *GetProposition) editorial(ctx context.Context, propositionID, actorID string) (TeamEditorial, error) {
	vs, err := uc.repos.Versions.ListByProposition(ctx, propositionID)
	if err != nil {
		return TeamEditorial{}, err
	}
	current, pending := domain.FindVersions(vs)
	var ids []string
	for _, v := range []*domain.Version{pending, current} {
		if v != nil {
			ids = append(ids, v.ID)
		}
	}
	ed := TeamEditorial{Versions: vs, Contents: map[string]domain.Content{}}
	if len(ids) > 0 {
		cs, err := uc.repos.Contents.ListByVersions(ctx, ids)
		if err != nil {
			return TeamEditorial{}, err
		}
		for _, c := range cs {
			ed.Contents[c.VersionID] = c
		}
	}
	topics, err := uc.repos.Topics.List(ctx)
	if err != nil {
		return TeamEditorial{}, err
	}
	ed.Topics = domain.TopicsByID(topics)
	if ed.Claim, err = uc.claims.Current(ctx, propositionID); err != nil {
		return TeamEditorial{}, err
	}
	if ed.Review, err = uc.reviews.Status(ctx, propositionID); err != nil {
		return TeamEditorial{}, err
	}
	// Editar exige a Posse livre ou do próprio Ator; o caso de uso de
	// gravar confere de novo.
	free := ed.Claim == nil || ed.Claim.UserID == actorID
	ed.CanEdit = pending != nil && free
	ed.CanCreateVersion = pending == nil && free
	ed.CanMinorFix = ed.Review.CanMinorFix && free

	ed.Structurings = map[string]StructuringView{}
	if len(ids) > 0 {
		sts, err := uc.repos.Structurings.ListByVersions(ctx, ids)
		if err != nil {
			return TeamEditorial{}, err
		}
		for _, st := range sts {
			ps, err := uc.repos.Structurings.ListProvisions(ctx, st.VersionID)
			if err != nil {
				return TeamEditorial{}, err
			}
			ed.Structurings[st.VersionID] = StructuringView{Structuring: st, Provisions: ps}
		}
	}
	ed.Comments = map[string][]domain.Comment{}
	if len(ids) > 0 {
		cs, err := uc.repos.Comments.ListByVersions(ctx, ids)
		if err != nil {
			return TeamEditorial{}, err
		}
		for _, c := range domain.LiveComments(cs) {
			ed.Comments[c.VersionID] = append(ed.Comments[c.VersionID], c)
		}
	}
	if pending != nil {
		if st, ok := ed.Structurings[pending.ID]; ok {
			ed.CanComment = ed.CanEdit && st.State == domain.StructuringConfirmed
			ed.CanEditStructuring = ed.CanEdit
			probe := st.Structuring
			ed.CanRetryStructuring = ed.CanEdit && probe.Retry(*pending, actorID, st.UpdatedAt) == nil
		}
	}

	var who []string
	for _, c := range ed.Contents {
		who = append(who, c.Authors...)
		if c.UpdatedBy != "" {
			who = append(who, c.UpdatedBy)
		}
	}
	for _, v := range vs {
		if v.CreatedBy != "" {
			who = append(who, v.CreatedBy)
		}
	}
	if ed.Claim != nil {
		who = append(who, ed.Claim.UserID)
	}
	for _, cs := range ed.Comments {
		for _, c := range cs {
			who = append(who, c.Authors...)
		}
	}
	for _, st := range ed.Structurings {
		if st.UpdatedBy != "" {
			who = append(who, st.UpdatedBy)
		}
		if st.ConfirmedBy != "" {
			who = append(who, st.ConfirmedBy)
		}
	}
	if a := ed.Review.Current; a != nil {
		who = append(who, a.ApprovedBy)
		if a.SecondReviewBy != "" {
			who = append(who, a.SecondReviewBy)
		}
	}
	for _, f := range ed.Review.MinorFixes {
		who = append(who, f.CreatedBy)
	}
	errata := ed.Review.Errata
	if e := ed.Review.PendingErratum; e != nil {
		errata = append([]ErratumInfo{*e}, errata...)
	}
	for _, e := range errata {
		who = append(who, e.CreatedBy)
		if e.DecidedBy != "" {
			who = append(who, e.DecidedBy)
		}
	}
	ed.People = map[string]string{}
	if len(who) > 0 {
		if ed.People, err = uc.people.Names(ctx, who); err != nil {
			return TeamEditorial{}, err
		}
	}
	return ed, nil
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
