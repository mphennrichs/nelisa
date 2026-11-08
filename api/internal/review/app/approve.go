package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

type ApproveInput struct {
	PropositionID string
	VersionID     string
	// Revision do conteúdo que o Revisor leu. Se mudou, a aprovação é
	// recusada: ninguém aprova texto que não viu.
	Revision int
}

// Approve marca a Versão em análise como pronta. Revisor ou Admin, sem
// exigir Posse; quatro olhos (o Admin pode furar, e fica a Segunda
// revisão pendente); conteúdo completo. Na mesma transação: a Versão vira
// a vigente, a anterior é substituída, a Proposição fica pronta, a Posse
// é solta e a Auditoria registra. Se qualquer passo falhar, nada fica.
type Approve struct {
	approvals ApprovalRepo
	editorial Editorial
	claims    Claims
	tx        TxRunner
	audit     AuditRecorder
	clock     clock.Clock
}

func NewApprove(approvals ApprovalRepo, editorial Editorial, claims Claims, tx TxRunner, audit AuditRecorder, clk clock.Clock) *Approve {
	return &Approve{approvals: approvals, editorial: editorial, claims: claims, tx: tx, audit: audit, clock: clk}
}

func (uc *Approve) Execute(ctx context.Context, in ApproveInput) (domain.Approval, error) {
	who, err := requireReviewer(ctx)
	if err != nil {
		return domain.Approval{}, err
	}
	var out domain.Approval
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		s, err := uc.editorial.LockPending(ctx, in.PropositionID, in.VersionID)
		if err != nil {
			return err
		}
		assessment := domain.Assess(candidate(&s), who)
		if err := assessment.Err(); err != nil {
			return err
		}
		if s.Revision != in.Revision {
			return domain.ErrStaleContent
		}
		a := domain.NewApproval(s.PropositionID, s.VersionID, assessment, who.UserID, uc.clock.Now())
		if err := uc.approvals.Create(ctx, a); err != nil {
			return err
		}
		if err := uc.editorial.Promote(ctx, s.PropositionID, s.VersionID); err != nil {
			return err
		}
		if err := uc.claims.Release(ctx, s.PropositionID); err != nil {
			return err
		}
		out = a
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: who.UserID, Action: ActionApprove, TargetType: targetVersion, TargetID: s.VersionID,
			Metadata: map[string]any{
				"proposition_id": s.PropositionID, "name": s.Name, "seq": s.VersionSeq, "four_eyes": a.FourEyes,
			},
		})
	})
	if err != nil {
		return domain.Approval{}, err
	}
	return out, nil
}

func candidate(s *Subject) domain.Candidate {
	if s == nil {
		return domain.Candidate{}
	}
	return domain.Candidate{HasPending: true, InReview: s.InReview, Authors: s.Authors, Missing: s.Missing}
}

type SecondReviewInput struct {
	PropositionID string
	VersionID     string
}

// SecondReview confirma a aprovação feita sem quatro olhos: Revisor ou
// Admin que não escreveu o conteúdo e não é quem aprovou. O Selo passa a
// "revisado por 2 pessoas". Só para a Versão vigente: de uma substituída
// o Leitor já não vê nada.
type SecondReview struct {
	approvals ApprovalRepo
	editorial Editorial
	tx        TxRunner
	audit     AuditRecorder
	clock     clock.Clock
}

func NewSecondReview(approvals ApprovalRepo, editorial Editorial, tx TxRunner, audit AuditRecorder, clk clock.Clock) *SecondReview {
	return &SecondReview{approvals: approvals, editorial: editorial, tx: tx, audit: audit, clock: clk}
}

func (uc *SecondReview) Execute(ctx context.Context, in SecondReviewInput) (domain.Approval, error) {
	who, err := requireReviewer(ctx)
	if err != nil {
		return domain.Approval{}, err
	}
	var out domain.Approval
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		v, err := uc.editorial.Version(ctx, in.PropositionID, in.VersionID)
		if err != nil {
			return err
		}
		a, err := uc.approvals.FindByVersionForUpdate(ctx, v.VersionID)
		if err != nil {
			return err
		}
		if !v.Current {
			return domain.ErrNoSecondReviewPending
		}
		if err := a.ConfirmSecondReview(who, v.Authors, uc.clock.Now()); err != nil {
			return err
		}
		if err := uc.approvals.SetSecondReview(ctx, a); err != nil {
			return err
		}
		out = a
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: who.UserID, Action: ActionSecondReview, TargetType: targetVersion, TargetID: v.VersionID,
			Metadata: map[string]any{"proposition_id": v.PropositionID, "seq": v.Seq, "approved_by": a.ApprovedBy},
		})
	})
	if err != nil {
		return domain.Approval{}, err
	}
	return out, nil
}
