package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

type ConfirmLinkInput struct {
	PropositionID string
	LinkID        string
}

// ConfirmLink é o Editor aceitando um Vínculo sugerido pelo sistema (ADR
// 0006). A Sincronização do novo Vínculo é enfileirada na mesma transação:
// só roda se a confirmação for gravada.
type ConfirmLink struct {
	links     LinkRepo
	tx        TxRunner
	audit     AuditRecorder
	scheduler SyncScheduler
	clock     clock.Clock
}

func NewConfirmLink(links LinkRepo, tx TxRunner, audit AuditRecorder, scheduler SyncScheduler, clk clock.Clock) *ConfirmLink {
	return &ConfirmLink{links: links, tx: tx, audit: audit, scheduler: scheduler, clock: clk}
}

func (uc *ConfirmLink) Execute(ctx context.Context, in ConfirmLinkInput) (domain.SourceLink, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return domain.SourceLink{}, err
	}
	var out domain.SourceLink
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		l, err := uc.links.FindByID(ctx, in.LinkID)
		if err != nil {
			return err
		}
		if l.PropositionID != in.PropositionID {
			return domain.ErrLinkNotFound
		}
		if err := l.Confirm(actor.UserID, uc.clock.Now()); err != nil {
			return err
		}
		// Conferência antecipada para um erro claro; os índices únicos do
		// banco são a garantia contra corrida.
		others, err := uc.links.ListByExternal(ctx, l.Chamber, []string{l.ExternalID})
		if err != nil {
			return err
		}
		for _, o := range others {
			if o.ID != l.ID && o.State == domain.LinkConfirmed {
				return domain.ErrLinkTaken
			}
		}
		siblings, err := uc.links.ListByProposition(ctx, l.PropositionID)
		if err != nil {
			return err
		}
		for _, s := range siblings {
			if s.ID != l.ID && s.Chamber == l.Chamber && s.State == domain.LinkConfirmed {
				return domain.ErrChamberAlreadyLinked
			}
		}
		if err := uc.links.Confirm(ctx, l); err != nil {
			return err
		}
		if err := uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionLinkConfirm, TargetType: targetLink, TargetID: l.ID,
			Metadata: map[string]any{"proposition_id": l.PropositionID, "chamber": string(l.Chamber), "external_id": l.ExternalID},
		}); err != nil {
			return err
		}
		out = l
		return uc.scheduler.ScheduleSync(ctx, l.ID)
	})
	if err != nil {
		return domain.SourceLink{}, err
	}
	return out, nil
}
