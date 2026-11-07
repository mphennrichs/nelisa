package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

type RejectLinkInput struct {
	PropositionID string
	LinkID        string
}

// RejectLink é o Editor descartando um Vínculo sugerido que não é a mesma
// Proposição. O vínculo some; a Auditoria guarda qual registro foi
// descartado.
type RejectLink struct {
	links LinkRepo
	tx    TxRunner
	audit AuditRecorder
}

func NewRejectLink(links LinkRepo, tx TxRunner, audit AuditRecorder) *RejectLink {
	return &RejectLink{links: links, tx: tx, audit: audit}
}

func (uc *RejectLink) Execute(ctx context.Context, in RejectLinkInput) error {
	actor, err := requireEditor(ctx)
	if err != nil {
		return err
	}
	return uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		l, err := uc.links.FindByID(ctx, in.LinkID)
		if err != nil {
			return err
		}
		if l.PropositionID != in.PropositionID {
			return domain.ErrLinkNotFound
		}
		if err := l.CanReject(); err != nil {
			return err
		}
		if err := uc.links.DeleteSuggested(ctx, l.ID); err != nil {
			return err
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionLinkReject, TargetType: targetLink, TargetID: l.ID,
			Metadata: map[string]any{"proposition_id": l.PropositionID, "chamber": string(l.Chamber), "external_id": l.ExternalID},
		})
	})
}
