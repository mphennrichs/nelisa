package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

// CorrectionDeps são as portas da Correção menor e da Errata.
type CorrectionDeps struct {
	Fixes   MinorFixRepo
	Errata  ErratumRepo
	Content Corrections
	Claims  ClaimGate
	Tx      TxRunner
	Audit   AuditRecorder
	Clock   clock.Clock
}

// --- Correção menor ---

type ApplyMinorFixInput struct {
	PropositionID string
	VersionID     string
	Summary       string
	ShortSummary  string
	TopicIDs      []string
	// Revision do conteúdo vigente que a pessoa leu (trava otimista).
	Revision int
}

// ApplyMinorFix aplica uma Correção menor ao conteúdo da Versão vigente
// de uma Proposição pronta. Editor, Revisor ou Admin, com a Posse (assume
// se livre); nenhuma Errata pendente na Versão. Entra na hora: o Leitor vê
// o texto corrigido, quem corrigiu entra na Autoria e a correção fica "a
// conferir". Correção, conteúdo e Auditoria andam juntos.
type ApplyMinorFix struct{ d CorrectionDeps }

func NewApplyMinorFix(d CorrectionDeps) *ApplyMinorFix { return &ApplyMinorFix{d: d} }

func (uc *ApplyMinorFix) Execute(ctx context.Context, in ApplyMinorFixInput) (domain.MinorFix, error) {
	who, err := approver(ctx)
	if err != nil {
		return domain.MinorFix{}, err
	}
	var out domain.MinorFix
	err = uc.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		t, err := uc.d.Content.LockCurrent(ctx, in.PropositionID, in.VersionID)
		if err != nil {
			return err
		}
		pending, err := uc.d.Errata.HasPending(ctx, t.VersionID)
		if err != nil {
			return err
		}
		if err := t.CanCorrect(pending); err != nil {
			return err
		}
		if err := uc.d.Claims.TouchForEdit(ctx, t.PropositionID, who.UserID); err != nil {
			return err
		}
		if t.Revision != in.Revision {
			return domain.ErrStaleContent
		}
		proposed, err := uc.d.Content.CheckContent(ctx, domain.Snapshot{
			Summary: in.Summary, ShortSummary: in.ShortSummary, TopicIDs: in.TopicIDs,
		}, t.Content.TopicIDs)
		if err != nil {
			return err
		}
		rw, err := uc.d.Content.Rewrite(ctx, t.PropositionID, t.VersionID, proposed, who.UserID, t.Revision)
		if err != nil {
			return err
		}
		id, err := newID()
		if err != nil {
			return err
		}
		// Sem campo mudado, NewMinorFix recusa e a transação desfaz a
		// renovação da Posse: abrir e salvar sem mexer não é correção.
		f, err := domain.NewMinorFix(id, t, rw.Fields, rw.Content, who.UserID, uc.d.Clock.Now())
		if err != nil {
			return err
		}
		if err := uc.d.Fixes.Create(ctx, f); err != nil {
			return err
		}
		out = f
		// Metadado só: Versão e campos, nunca o texto.
		return uc.d.Audit.Record(ctx, AuditEvent{
			ActorID: who.UserID, Action: ActionMinorFix, TargetType: targetVersion, TargetID: t.VersionID,
			Metadata: map[string]any{
				"proposition_id": t.PropositionID, "seq": t.Seq, "fields": f.Fields,
				"revision": rw.Revision, "minor_fix_id": f.ID,
			},
		})
	})
	if err != nil {
		return domain.MinorFix{}, err
	}
	return out, nil
}

type CheckMinorFixInput struct {
	PropositionID string
	MinorFixID    string
}

// CheckMinorFix é a conferência da Correção menor por um Revisor ou Admin
// que não a fez. Não há "rejeitar": quem discorda faz outra Correção menor
// ou propõe uma Errata.
type CheckMinorFix struct{ d CorrectionDeps }

func NewCheckMinorFix(d CorrectionDeps) *CheckMinorFix { return &CheckMinorFix{d: d} }

func (uc *CheckMinorFix) Execute(ctx context.Context, in CheckMinorFixInput) (domain.MinorFix, error) {
	who, err := requireReviewer(ctx)
	if err != nil {
		return domain.MinorFix{}, err
	}
	var out domain.MinorFix
	err = uc.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		f, err := uc.d.Fixes.FindForUpdate(ctx, in.MinorFixID)
		if err != nil {
			return err
		}
		if f.PropositionID != in.PropositionID {
			return domain.ErrMinorFixNotFound
		}
		if err := f.Check(who, uc.d.Clock.Now()); err != nil {
			return err
		}
		if err := uc.d.Fixes.SetChecked(ctx, f); err != nil {
			return err
		}
		out = f
		return uc.d.Audit.Record(ctx, AuditEvent{
			ActorID: who.UserID, Action: ActionMinorFixCheck, TargetType: targetMinorFix, TargetID: f.ID,
			Metadata: map[string]any{"proposition_id": f.PropositionID, "version_id": f.VersionID, "fixed_by": f.CreatedBy},
		})
	})
	if err != nil {
		return domain.MinorFix{}, err
	}
	return out, nil
}

// --- Errata ---

type ProposeErratumInput struct {
	PropositionID string
	VersionID     string
	Summary       string
	ShortSummary  string
	TopicIDs      []string
	Said          string
	Correct       string
	// Revision do conteúdo vigente que a pessoa leu.
	Revision int
}

// ProposeErratum propõe uma Errata para a Versão vigente de uma
// Proposição pronta. Editor, Revisor ou Admin; não exige Posse (nada muda
// até a aprovação). No máximo uma pendente por Versão.
type ProposeErratum struct{ d CorrectionDeps }

func NewProposeErratum(d CorrectionDeps) *ProposeErratum { return &ProposeErratum{d: d} }

func (uc *ProposeErratum) Execute(ctx context.Context, in ProposeErratumInput) (domain.Erratum, error) {
	who, err := approver(ctx)
	if err != nil {
		return domain.Erratum{}, err
	}
	var out domain.Erratum
	err = uc.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		// Trava a Proposição: serializa com a Correção menor, que confere
		// se há Errata pendente.
		t, err := uc.d.Content.LockCurrent(ctx, in.PropositionID, in.VersionID)
		if err != nil {
			return err
		}
		pending, err := uc.d.Errata.HasPending(ctx, t.VersionID)
		if err != nil {
			return err
		}
		if err := t.CanCorrect(pending); err != nil {
			return err
		}
		if t.Revision != in.Revision {
			return domain.ErrStaleContent
		}
		proposed, err := uc.d.Content.CheckContent(ctx, domain.Snapshot{
			Summary: in.Summary, ShortSummary: in.ShortSummary, TopicIDs: in.TopicIDs,
		}, t.Content.TopicIDs)
		if err != nil {
			return err
		}
		id, err := newID()
		if err != nil {
			return err
		}
		e, err := domain.NewErratum(id, t, proposed, in.Said, in.Correct, who.UserID, uc.d.Clock.Now())
		if err != nil {
			return err
		}
		if err := uc.d.Errata.Create(ctx, e); err != nil {
			return err
		}
		out = e
		return uc.d.Audit.Record(ctx, AuditEvent{
			ActorID: who.UserID, Action: ActionErratumPropose, TargetType: targetErratum, TargetID: e.ID,
			Metadata: map[string]any{"proposition_id": t.PropositionID, "version_id": t.VersionID, "seq": t.Seq, "base_revision": e.BaseRevision},
		})
	})
	if err != nil {
		return domain.Erratum{}, err
	}
	return out, nil
}

// ErratumRef aponta uma Errata pela URL: Proposição, Versão e id.
type ErratumRef struct {
	PropositionID string
	VersionID     string
	ErratumID     string
}

// find trava a Errata e confere que ela é da Proposição e da Versão da
// URL: id de outra Proposição é "não encontrada", sem revelar que existe.
func (d CorrectionDeps) find(ctx context.Context, ref ErratumRef) (domain.Erratum, error) {
	e, err := d.Errata.FindForUpdate(ctx, ref.ErratumID)
	if err != nil {
		return domain.Erratum{}, err
	}
	if e.PropositionID != ref.PropositionID || e.VersionID != ref.VersionID {
		return domain.Erratum{}, domain.ErrErratumNotFound
	}
	return e, nil
}

type ApproveErratumInput struct {
	ErratumRef
	// Revision do conteúdo vigente que quem aprova leu: tem de ser a mesma
	// de quando a Errata foi proposta.
	Revision int
}

// ApproveErratum aprova a Errata: Revisor ou Admin que não a propôs (sem
// exceção para o Admin). Na mesma transação o conteúdo vigente vira o da
// Errata (quem propôs entra na Autoria), a Errata fica aprovada e a
// Auditoria registra. Se o conteúdo vigente mudou desde a proposta, 409:
// a Errata precisa ser refeita.
type ApproveErratum struct{ d CorrectionDeps }

func NewApproveErratum(d CorrectionDeps) *ApproveErratum { return &ApproveErratum{d: d} }

func (uc *ApproveErratum) Execute(ctx context.Context, in ApproveErratumInput) (domain.Erratum, error) {
	who, err := requireReviewer(ctx)
	if err != nil {
		return domain.Erratum{}, err
	}
	var out domain.Erratum
	err = uc.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		// A Proposição primeiro, como em toda escrita de conteúdo: mesma
		// ordem de travas, sem impasse.
		t, err := uc.d.Content.LockCurrent(ctx, in.PropositionID, in.VersionID)
		if err != nil {
			return err
		}
		e, err := uc.d.find(ctx, in.ErratumRef)
		if err != nil {
			return err
		}
		if err := e.CanDecide(who); err != nil {
			return err
		}
		if !t.Ready {
			return domain.ErrNotReady
		}
		if in.Revision != e.BaseRevision || e.Stale(t.Revision) {
			return domain.ErrStaleContent
		}
		// Valida de novo: um Tema escolhido pode ter sido desativado
		// enquanto a Errata esperava.
		proposed, err := uc.d.Content.CheckContent(ctx, e.Proposed, t.Content.TopicIDs)
		if err != nil {
			return err
		}
		rw, err := uc.d.Content.Rewrite(ctx, t.PropositionID, t.VersionID, proposed, e.CreatedBy, t.Revision)
		if err != nil {
			return err
		}
		if err := e.Approve(who, uc.d.Clock.Now()); err != nil {
			return err
		}
		if err := uc.d.Errata.Decide(ctx, e); err != nil {
			return err
		}
		out = e
		return uc.d.Audit.Record(ctx, AuditEvent{
			ActorID: who.UserID, Action: ActionErratumApprove, TargetType: targetErratum, TargetID: e.ID,
			Metadata: map[string]any{
				"proposition_id": t.PropositionID, "version_id": t.VersionID, "seq": t.Seq,
				"fields": rw.Fields, "revision": rw.Revision, "proposed_by": e.CreatedBy,
			},
		})
	})
	if err != nil {
		return domain.Erratum{}, err
	}
	return out, nil
}

type RejectErratumInput struct {
	ErratumRef
	Reason string
}

// RejectErratum recusa a Errata com motivo interno: Revisor ou Admin que
// não a propôs. Nada muda no conteúdo.
type RejectErratum struct{ d CorrectionDeps }

func NewRejectErratum(d CorrectionDeps) *RejectErratum { return &RejectErratum{d: d} }

func (uc *RejectErratum) Execute(ctx context.Context, in RejectErratumInput) (domain.Erratum, error) {
	who, err := requireReviewer(ctx)
	if err != nil {
		return domain.Erratum{}, err
	}
	var out domain.Erratum
	err = uc.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		e, err := uc.d.find(ctx, in.ErratumRef)
		if err != nil {
			return err
		}
		if err := e.Reject(who, in.Reason, uc.d.Clock.Now()); err != nil {
			return err
		}
		if err := uc.d.Errata.Decide(ctx, e); err != nil {
			return err
		}
		out = e
		return uc.d.Audit.Record(ctx, AuditEvent{
			ActorID: who.UserID, Action: ActionErratumReject, TargetType: targetErratum, TargetID: e.ID,
			Metadata: map[string]any{"proposition_id": e.PropositionID, "version_id": e.VersionID, "proposed_by": e.CreatedBy},
		})
	})
	if err != nil {
		return domain.Erratum{}, err
	}
	return out, nil
}

// WithdrawErratum é quem propôs desistindo da Errata pendente. Nada muda
// no conteúdo.
type WithdrawErratum struct{ d CorrectionDeps }

func NewWithdrawErratum(d CorrectionDeps) *WithdrawErratum { return &WithdrawErratum{d: d} }

func (uc *WithdrawErratum) Execute(ctx context.Context, in ErratumRef) (domain.Erratum, error) {
	who, err := approver(ctx)
	if err != nil {
		return domain.Erratum{}, err
	}
	var out domain.Erratum
	err = uc.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		e, err := uc.d.find(ctx, in)
		if err != nil {
			return err
		}
		if err := e.Withdraw(who.UserID, uc.d.Clock.Now()); err != nil {
			return err
		}
		if err := uc.d.Errata.Decide(ctx, e); err != nil {
			return err
		}
		out = e
		return uc.d.Audit.Record(ctx, AuditEvent{
			ActorID: who.UserID, Action: ActionErratumWithdraw, TargetType: targetErratum, TargetID: e.ID,
			Metadata: map[string]any{"proposition_id": e.PropositionID, "version_id": e.VersionID},
		})
	})
	if err != nil {
		return domain.Erratum{}, err
	}
	return out, nil
}
