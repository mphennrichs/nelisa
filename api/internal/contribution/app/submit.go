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
//
// Antes de olhar o formulário, o envio passa pelo anti-abuso, do mais
// barato ao mais caro: o teto de pendentes da Fila e o limite por IP.
type Submit struct {
	contributions ContributionRepo
	sources       OfficialSourceRepo
	props         Propositions
	availability  *Availability
	limiter       *RateLimiter
	perHour       int
	clock         clock.Clock
	random        io.Reader
}

// SubmitLimits são os limites do anti-abuso (config).
type SubmitLimits struct {
	// PerHour é quantos envios um IP faz por hora.
	PerHour int
}

func NewSubmit(c ContributionRepo, s OfficialSourceRepo, p Propositions, av *Availability, limiter *RateLimiter, limits SubmitLimits, clk clock.Clock, random io.Reader) *Submit {
	return &Submit{
		contributions: c, sources: s, props: p, availability: av, limiter: limiter,
		perHour: limits.PerHour, clock: clk, random: random,
	}
}

// SubmitInput é o formulário. PropositionSlug só na Atualização de
// tramitação. ClientKey é o IP de quem manda (ou o /64 no IPv6): só serve
// ao rate limit e nunca é gravado.
type SubmitInput struct {
	domain.Submission
	PropositionSlug string
	ClientKey       string
}

func (uc *Submit) Execute(ctx context.Context, in SubmitInput) (domain.TrackingCode, error) {
	if err := uc.availability.Execute(ctx); err != nil {
		return "", err
	}
	if err := uc.limiter.Allow(ctx, domain.BucketSubmit, in.ClientKey, uc.perHour); err != nil {
		return "", err
	}
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
	limiter       *RateLimiter
	perHour       int
}

func NewLookup(c ContributionRepo, p Propositions, limiter *RateLimiter, perHour int) *Lookup {
	return &Lookup{contributions: c, props: p, limiter: limiter, perHour: perHour}
}

// LookupInput é o código digitado e o IP de quem consulta (só para o rate
// limit, que impede varrer códigos).
type LookupInput struct {
	Code      string
	ClientKey string
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

func (uc *Lookup) Execute(ctx context.Context, in LookupInput) (LookupResult, error) {
	if err := uc.limiter.Allow(ctx, domain.BucketLookup, in.ClientKey, uc.perHour); err != nil {
		return LookupResult{}, err
	}
	code, err := domain.ParseTrackingCode(in.Code)
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
