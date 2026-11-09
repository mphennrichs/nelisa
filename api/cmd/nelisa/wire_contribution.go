package main

import (
	"context"

	auditapp "github.com/mphennrichs/nelisa/api/internal/audit/app"
	contribapp "github.com/mphennrichs/nelisa/api/internal/contribution/app"
	contribdomain "github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	contribhttp "github.com/mphennrichs/nelisa/api/internal/contribution/httpapi"
	"github.com/mphennrichs/nelisa/api/internal/platform/clientip"
	"github.com/mphennrichs/nelisa/api/internal/platform/config"
	propapp "github.com/mphennrichs/nelisa/api/internal/proposition/app"
	propdomain "github.com/mphennrichs/nelisa/api/internal/proposition/domain"
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

type contribAudit struct{ record recordAudit }

func (a contribAudit) Record(ctx context.Context, e contribapp.AuditEvent) error {
	return a.record(ctx, auditapp.RecordEventInput{
		ActorID: e.ActorID, Action: e.Action, TargetType: e.TargetType, TargetID: e.TargetID, Metadata: e.Metadata,
	})
}
