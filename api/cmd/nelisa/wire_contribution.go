package main

import (
	"context"
	"errors"

	auditapp "github.com/mphennrichs/nelisa/api/internal/audit/app"
	contribapp "github.com/mphennrichs/nelisa/api/internal/contribution/app"
	contribdomain "github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	contribhttp "github.com/mphennrichs/nelisa/api/internal/contribution/httpapi"
	"github.com/mphennrichs/nelisa/api/internal/platform/clientip"
	"github.com/mphennrichs/nelisa/api/internal/platform/config"
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
		Enabled:  cfg.ContributionsEnabled,
		LimitKey: clientip.New(cfg.TrustedProxies).LimitKey,
	}
}

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
