package app

import (
	"context"
	"errors"

	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

// Reports são as consultas que os outros módulos fazem à Revisão, ligadas
// no main: a situação de uma Proposição para a equipe (com a decisão de
// quem pode aprovar), o Selo para a camada pública e as Segundas revisões
// pendentes para a Fila. Só leem.
type Reports struct {
	approvals   ApprovalRepo
	fixes       MinorFixRepo
	errata      ErratumRepo
	editorial   Editorial
	corrections Corrections
}

func NewReports(approvals ApprovalRepo, fixes MinorFixRepo, errata ErratumRepo, editorial Editorial, corrections Corrections) *Reports {
	return &Reports{approvals: approvals, fixes: fixes, errata: errata, editorial: editorial, corrections: corrections}
}

// Status é a Revisão de uma Proposição vista pelo Ator do contexto.
type Status struct {
	Assessment domain.Assessment
	// CanSecondReview: o Ator pode confirmar a segunda revisão da vigente.
	CanSecondReview bool
	// Current é a aprovação da Versão vigente (nil se não há vigente).
	Current *domain.Approval

	// CanMinorFix e CanProposeErratum: a Versão vigente aceita Correção
	// menor e Errata agora (Proposição pronta, sem Errata pendente). A
	// Posse, que a Correção menor também exige, é decidida por quem tem
	// essa informação (o módulo proposition, com o queue).
	CanMinorFix       bool
	CanProposeErratum bool
	// MinorFixes são as Correções menores a conferir.
	MinorFixes []MinorFixView
	// PendingErratum é a Errata pendente da Versão vigente.
	PendingErratum *ErratumView
	// Errata são as outras Erratas da Proposição (decididas, ou pendentes
	// de uma Versão que já não é a vigente), da mais nova para a mais
	// antiga.
	Errata []ErratumView
}

// MinorFixView é uma Correção menor com a decisão do Ator.
type MinorFixView struct {
	domain.MinorFix
	CanCheck bool
}

// ErratumView é uma Errata com as decisões do Ator.
type ErratumView struct {
	domain.Erratum
	// Stale: o conteúdo vigente mudou desde a proposta; aprovar dá 409.
	Stale       bool
	CanDecide   bool
	CanWithdraw bool
}

// Status decide, para o Ator do contexto, se ele pode aprovar a Versão em
// análise (e por que não) e se pode fazer a segunda revisão da vigente.
func (r *Reports) Status(ctx context.Context, propositionID string) (Status, error) {
	who, err := approver(ctx)
	if err != nil {
		return Status{}, err
	}
	s, err := r.editorial.Pending(ctx, propositionID)
	if err != nil {
		return Status{}, err
	}
	out := Status{Assessment: domain.Assess(candidate(s), who)}
	if err := r.approvalStatus(ctx, &out, who, propositionID); err != nil {
		return Status{}, err
	}
	if err := r.correctionStatus(ctx, &out, who, propositionID); err != nil {
		return Status{}, err
	}
	return out, nil
}

func (r *Reports) approvalStatus(ctx context.Context, out *Status, who domain.Approver, propositionID string) error {
	currentID, err := r.editorial.CurrentVersionID(ctx, propositionID)
	if err != nil || currentID == "" {
		return err
	}
	a, err := r.approvals.FindByVersion(ctx, currentID)
	if errors.Is(err, domain.ErrApprovalNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	out.Current = &a
	if a.PendingSecondReview() {
		v, err := r.editorial.Version(ctx, propositionID, currentID)
		if err != nil {
			return err
		}
		out.CanSecondReview = a.CanSecondReview(who, v.Authors) == nil
	}
	return nil
}

// correctionStatus decide o que o Ator pode fazer com Correções menores e
// Erratas: as mesmas regras dos casos de uso, aplicadas sem gravar.
func (r *Reports) correctionStatus(ctx context.Context, out *Status, who domain.Approver, propositionID string) error {
	t, err := r.corrections.Current(ctx, propositionID)
	if err != nil {
		return err
	}
	fixes, err := r.fixes.ListUncheckedByProposition(ctx, propositionID)
	if err != nil {
		return err
	}
	out.MinorFixes = make([]MinorFixView, 0, len(fixes))
	for _, f := range fixes {
		out.MinorFixes = append(out.MinorFixes, MinorFixView{MinorFix: f, CanCheck: f.CanCheck(who) == nil})
	}
	errata, err := r.errata.ListByProposition(ctx, propositionID)
	if err != nil {
		return err
	}
	out.Errata = []ErratumView{}
	pendingOnCurrent := false
	for _, e := range errata {
		onCurrent := t != nil && e.VersionID == t.VersionID
		v := ErratumView{Erratum: e, CanWithdraw: e.CanWithdraw(who.UserID) == nil}
		if e.Pending() && onCurrent {
			pendingOnCurrent = true
			v.Stale = e.Stale(t.Revision)
			// Aprovar exige a Proposição pronta; recusar não, mas a tela
			// mostra as duas decisões juntas.
			v.CanDecide = t.Ready && e.CanDecide(who) == nil
			out.PendingErratum = &v
			continue
		}
		out.Errata = append(out.Errata, v)
	}
	if t != nil && t.CanCorrect(pendingOnCurrent) == nil {
		out.CanMinorFix, out.CanProposeErratum = true, true
	}
	return nil
}

// UncheckedMinorFixes lista as Correções menores a conferir, da mais
// antiga para a mais nova (Fila).
func (r *Reports) UncheckedMinorFixes(ctx context.Context) ([]domain.MinorFix, error) {
	return r.fixes.ListUnchecked(ctx)
}

// PendingErrata lista as Erratas pendentes, da mais antiga para a mais
// nova (Fila).
func (r *Reports) PendingErrata(ctx context.Context) ([]domain.Erratum, error) {
	return r.errata.ListPending(ctx)
}

// ApprovedErrata lista as Erratas aprovadas das Versões pedidas, da mais
// nova para a mais antiga (camada pública). Só as aprovadas: pendente,
// recusada e retirada nunca saem daqui.
func (r *Reports) ApprovedErrata(ctx context.Context, versionIDs []string) ([]domain.Erratum, error) {
	if len(versionIDs) == 0 {
		return nil, nil
	}
	es, err := r.errata.ListApprovedByVersions(ctx, versionIDs)
	if err != nil {
		return nil, err
	}
	out := es[:0]
	for _, e := range es {
		if e.State == domain.ErratumApproved {
			out = append(out, e)
		}
	}
	return out, nil
}

// Badges devolve o Selo (1 ou 2) de cada Versão aprovada entre as pedidas.
func (r *Reports) Badges(ctx context.Context, versionIDs []string) (map[string]int, error) {
	out := map[string]int{}
	if len(versionIDs) == 0 {
		return out, nil
	}
	as, err := r.approvals.ListByVersions(ctx, versionIDs)
	if err != nil {
		return nil, err
	}
	for _, a := range as {
		out[a.VersionID] = a.Badge()
	}
	return out, nil
}

// PendingSecondReviews lista as aprovações sem quatro olhos que ainda
// esperam a segunda revisão, da mais antiga para a mais nova.
func (r *Reports) PendingSecondReviews(ctx context.Context) ([]domain.Approval, error) {
	return r.approvals.ListPendingSecondReview(ctx)
}
