package app

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

// Submit é o Leitor mandando uma Contribuição. Não há Ator nem Auditoria:
// quem manda é anônimo, e a Contribuição não muda nada até a equipe
// decidir.
type Submit struct {
	contributions ContributionRepo
	sources       OfficialSourceRepo
	props         Propositions
	clock         clock.Clock
	random        io.Reader
}

func NewSubmit(c ContributionRepo, s OfficialSourceRepo, p Propositions, clk clock.Clock, random io.Reader) *Submit {
	return &Submit{contributions: c, sources: s, props: p, clock: clk, random: random}
}

// SubmitInput é o formulário. PropositionSlug só na Atualização de
// tramitação.
type SubmitInput struct {
	domain.Submission
	PropositionSlug string
}

func (uc *Submit) Execute(ctx context.Context, in SubmitInput) (domain.TrackingCode, error) {
	c := domain.Context{Now: uc.clock.Now()}
	if in.Kind == domain.KindProgressUpdate && in.PropositionSlug != "" {
		p, err := uc.props.PublicBySlug(ctx, in.PropositionSlug)
		if err != nil {
			return "", err
		}
		if p != nil {
			c.Proposition = &domain.PropositionRef{ID: p.ID, Sphere: p.Sphere}
		}
	}
	if in.Kind == domain.KindIndication || in.Kind == domain.KindSourceRequest {
		ds, err := uc.sources.Domains(ctx)
		if err != nil {
			return "", err
		}
		c.Registered = ds
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	// Código repetido em 60 bits não acontece; ainda assim, sorteia de
	// novo em vez de falhar.
	for range 3 {
		code, err := domain.NewTrackingCode(uc.random)
		if err != nil {
			return "", err
		}
		contrib, err := domain.NewContribution(id, code, in.Submission, c)
		if err != nil {
			return "", err
		}
		err = uc.contributions.Insert(ctx, contrib)
		if errors.Is(err, ErrTrackingCodeTaken) {
			continue
		}
		if err != nil {
			return "", err
		}
		return code, nil
	}
	return "", ErrTrackingCodeTaken
}

// Lookup é o Leitor consultando pelo código de acompanhamento.
type Lookup struct {
	contributions ContributionRepo
	props         Propositions
}

func NewLookup(c ContributionRepo, p Propositions) *Lookup {
	return &Lookup{contributions: c, props: p}
}

// LookupResult é tudo o que o Leitor vê: nunca o texto enviado nem quem
// decidiu.
type LookupResult struct {
	Kind         domain.Kind
	State        domain.State
	SubmittedAt  time.Time
	DecidedAt    *time.Time
	PublicReason string
	// PropositionSlug: a Proposição da Atualização, ou a criada a partir da
	// Indicação aceita, se for pública ("" senão).
	PropositionSlug string
}

func (uc *Lookup) Execute(ctx context.Context, raw string) (LookupResult, error) {
	code, err := domain.ParseTrackingCode(raw)
	if err != nil {
		// Código malformado e código que não existe dão a mesma resposta.
		return LookupResult{}, domain.ErrContributionNotFound
	}
	c, err := uc.contributions.FindByTrackingHash(ctx, code.Hash())
	if err != nil {
		return LookupResult{}, err
	}
	out := LookupResult{
		Kind: c.Kind, State: c.State, SubmittedAt: c.SubmittedAt, DecidedAt: c.DecidedAt,
	}
	if c.State == domain.StateRejected {
		out.PublicReason = c.PublicReason
	}
	propID := c.PropositionID
	if c.Kind == domain.KindIndication {
		propID = ""
		if c.State == domain.StateAccepted {
			propID = c.CreatedPropositionID
		}
	}
	if propID != "" {
		p, err := uc.props.PublicByID(ctx, propID)
		if err != nil {
			return LookupResult{}, err
		}
		if p != nil {
			out.PropositionSlug = p.Slug
		}
	}
	return out, nil
}
