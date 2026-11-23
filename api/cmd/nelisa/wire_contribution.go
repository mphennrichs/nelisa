package main

import (
	"context"
	"errors"
	"fmt"

	auditapp "github.com/mphennrichs/nelisa/api/internal/audit/app"
	contribapp "github.com/mphennrichs/nelisa/api/internal/contribution/app"
	contribdomain "github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	contribhttp "github.com/mphennrichs/nelisa/api/internal/contribution/httpapi"
	"github.com/mphennrichs/nelisa/api/internal/platform/clientip"
	"github.com/mphennrichs/nelisa/api/internal/platform/config"
	"github.com/mphennrichs/nelisa/api/internal/platform/turnstile"
	progressapp "github.com/mphennrichs/nelisa/api/internal/progress/app"
	progressdomain "github.com/mphennrichs/nelisa/api/internal/progress/domain"
	propapp "github.com/mphennrichs/nelisa/api/internal/proposition/app"
	propdomain "github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	queueapp "github.com/mphennrichs/nelisa/api/internal/queue/app"
	queuedomain "github.com/mphennrichs/nelisa/api/internal/queue/domain"
)

// O módulo contribution não conhece o proposition: só pergunta, pela porta
// Propositions, se uma Proposição é pública. Este arquivo liga os dois.

// contributionOptions são as opções HTTP do módulo, vindas da config.
func contributionOptions(cfg config.Config) contribhttp.Options {
	return contribhttp.Options{
		Enabled:          cfg.ContributionsEnabled,
		LimitKey:         clientip.New(cfg.TrustedProxies).LimitKey,
		TurnstileSiteKey: cfg.TurnstileSiteKey,
	}
}

// lookupPerHour é o limite por IP da consulta pelo código: fixo, alto o
// bastante para o Leitor e baixo para quem tenta varrer códigos.
const lookupPerHour = 30

// turnstileAction é a ação que o widget do formulário declara; o Go
// confere que o token veio dele.
const turnstileAction = "contribute"

// contribAntiAbuse é o anti-abuso do envio, vindo da config: limites e o
// captcha.
type contribAntiAbuse struct {
	perHour, pendingCap int
	human               contribapp.HumanCheck
}

// contributionAntiAbuse monta o anti-abuso. opts ajustam o Turnstile (nos
// testes, um siteverify local).
func contributionAntiAbuse(cfg config.Config, opts ...turnstile.Option) contribAntiAbuse {
	host := ""
	if cfg.PublicBaseURL != nil {
		host = cfg.PublicBaseURL.Hostname()
	}
	return contribAntiAbuse{
		perHour: cfg.ContributionRatePerHour, pendingCap: cfg.ContributionPendingCap,
		human: contribHuman{v: turnstile.New(cfg.TurnstileSecret, host, turnstileAction, opts...)},
	}
}

// contribHuman liga a porta HumanCheck ao Turnstile, traduzindo os erros.
type contribHuman struct{ v *turnstile.Verifier }

func (h contribHuman) Verify(ctx context.Context, token string) error {
	err := h.v.Verify(ctx, token)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, turnstile.ErrRejected):
		return fmt.Errorf("%w: %v", contribapp.ErrHumanCheckFailed, err)
	default:
		// Fora do ar ou mal configurado: recusa, nunca aprova.
		return fmt.Errorf("%w: %v", contribapp.ErrHumanCheckUnavailable, err)
	}
}

// noHuman recusa todo envio: é o captcha do worker, que não recebe
// Contribuições.
type noHuman struct{}

func (noHuman) Verify(context.Context, string) error { return contribapp.ErrHumanCheckUnavailable }

// contribProps liga a porta Propositions do contribution ao Catalog do
// proposition (que aplica a regra de visibilidade).
type contribProps struct{ c *propapp.Catalog }

func toPublicProposition(p propdomain.Proposition, ok bool, err error) (*contribapp.PublicProposition, error) {
	if err != nil || !ok {
		return nil, err
	}
	return &contribapp.PublicProposition{ID: p.ID, Slug: p.Slug, Sphere: contribdomain.Sphere(p.Sphere)}, nil
}

func (a contribProps) PublicBySlug(ctx context.Context, slug string) (*contribapp.PublicProposition, error) {
	return toPublicProposition(a.c.PublicBySlug(ctx, slug))
}

func (a contribProps) PublicByID(ctx context.Context, id string) (*contribapp.PublicProposition, error) {
	return toPublicProposition(a.c.PublicByID(ctx, id))
}

func (a contribProps) ByID(ctx context.Context, id string) (*contribapp.TeamProposition, error) {
	p, err := a.c.Proposition(ctx, id)
	if errors.Is(err, propdomain.ErrPropositionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &contribapp.TeamProposition{ID: p.ID, Name: p.Name(), Slug: p.Slug, Sphere: contribdomain.Sphere(p.Sphere)}, nil
}

// contribProgress liga a porta ProgressEntries do contribution à
// Tramitação (módulo progress).
type contribProgress struct{ uc *progressapp.ManualEntries }

func (a contribProgress) RecordManual(ctx context.Context, e contribapp.ManualEvent) error {
	err := a.uc.Record(ctx, progressapp.ManualEntryInput{
		PropositionID: e.PropositionID, Chamber: e.Chamber, ContributionID: e.ContributionID,
		At: e.At, Description: e.Description, SourceURL: e.SourceURL,
	})
	if errors.Is(err, progressdomain.ErrManualEntryExists) {
		return contribapp.ErrProgressEntryExists
	}
	return err
}

func (a contribProgress) DeleteManual(ctx context.Context, contributionID string) (bool, error) {
	return a.uc.Delete(ctx, contributionID)
}

// queueContributions liga a porta Contributions da Fila às Contribuições
// pendentes.
type queueContributions struct{ uc *contribapp.ListPending }

func (q queueContributions) Pending(ctx context.Context) ([]queueapp.PendingContribution, error) {
	cs, err := q.uc.Execute(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]queueapp.PendingContribution, 0, len(cs))
	for _, c := range cs {
		out = append(out, queueapp.PendingContribution{
			ID: c.ID, Kind: string(c.Kind), Sphere: queuedomain.Sphere(c.Sphere), PropositionID: c.PropositionID,
			Subject: c.Subject(), SubmittedAt: c.SubmittedAt,
		})
	}
	return out, nil
}

type contribAudit struct{ record recordAudit }

func (a contribAudit) Record(ctx context.Context, e contribapp.AuditEvent) error {
	return a.record(ctx, auditapp.RecordEventInput{
		ActorID: e.ActorID, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID, Metadata: e.Metadata,
	})
}
