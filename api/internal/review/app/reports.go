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
	approvals ApprovalRepo
	editorial Editorial
}

func NewReports(approvals ApprovalRepo, editorial Editorial) *Reports {
	return &Reports{approvals: approvals, editorial: editorial}
}

// Status é a Revisão de uma Proposição vista pelo Ator do contexto.
type Status struct {
	Assessment domain.Assessment
	// CanSecondReview: o Ator pode confirmar a segunda revisão da vigente.
	CanSecondReview bool
	// Current é a aprovação da Versão vigente (nil se não há vigente).
	Current *domain.Approval
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
	currentID, err := r.editorial.CurrentVersionID(ctx, propositionID)
	if err != nil || currentID == "" {
		return out, err
	}
	a, err := r.approvals.FindByVersion(ctx, currentID)
	if errors.Is(err, domain.ErrApprovalNotFound) {
		return out, nil
	}
	if err != nil {
		return Status{}, err
	}
	out.Current = &a
	if a.PendingSecondReview() {
		v, err := r.editorial.Version(ctx, propositionID, currentID)
		if err != nil {
			return Status{}, err
		}
		out.CanSecondReview = a.CanSecondReview(who, v.Authors) == nil
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
