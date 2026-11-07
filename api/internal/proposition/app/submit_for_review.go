package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

type SubmitForReviewInput struct {
	PropositionID string
}

// SubmitForReview leva a Proposição de "criado" para "em revisão": a parte
// pública passa a mostrá-la "em análise", só com dados oficiais. Marcar
// como pronto é da fase 2 (Revisor, quatro olhos).
type SubmitForReview struct {
	props PropositionRepo
	tx    TxRunner
	audit AuditRecorder
	clock clock.Clock
}

func NewSubmitForReview(props PropositionRepo, tx TxRunner, audit AuditRecorder, clk clock.Clock) *SubmitForReview {
	return &SubmitForReview{props: props, tx: tx, audit: audit, clock: clk}
}

func (uc *SubmitForReview) Execute(ctx context.Context, in SubmitForReviewInput) (domain.Proposition, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return domain.Proposition{}, err
	}
	var out domain.Proposition
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := uc.props.FindByIDForUpdate(ctx, in.PropositionID)
		if err != nil {
			return err
		}
		from := p.ReviewState
		if err := p.SubmitForReview(uc.clock.Now()); err != nil {
			return err
		}
		if err := uc.props.Update(ctx, p); err != nil {
			return err
		}
		out = p
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionSubmitForReview, TargetType: targetProposition, TargetID: p.ID,
			Metadata: map[string]any{"from": string(from), "to": string(p.ReviewState), "name": p.Name()},
		})
	})
	if err != nil {
		return domain.Proposition{}, err
	}
	return out, nil
}
