package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

// ListOfficialSources: Editor ou acima vê os domínios cadastrados.
type ListOfficialSources struct{ sources OfficialSourceRepo }

func NewListOfficialSources(s OfficialSourceRepo) *ListOfficialSources {
	return &ListOfficialSources{sources: s}
}

func (uc *ListOfficialSources) Execute(ctx context.Context) ([]domain.OfficialSource, error) {
	if _, err := requireRole(ctx, auth.RoleEditor); err != nil {
		return nil, err
	}
	return uc.sources.List(ctx)
}

// CreateOfficialSource: Editor ou Revisor cadastra um domínio como Fonte
// oficial (CONTEXT.md). Domínio já coberto (.leg.br, .gov.br ou um domínio
// cadastrado acima dele) é recusado.
type CreateOfficialSource struct {
	sources OfficialSourceRepo
	tx      TxRunner
	audit   AuditRecorder
	clock   clock.Clock
}

func NewCreateOfficialSource(s OfficialSourceRepo, tx TxRunner, a AuditRecorder, clk clock.Clock) *CreateOfficialSource {
	return &CreateOfficialSource{sources: s, tx: tx, audit: a, clock: clk}
}

type CreateOfficialSourceInput struct {
	Domain string
	Name   string
	Sphere domain.Sphere
}

func (uc *CreateOfficialSource) Execute(ctx context.Context, in CreateOfficialSourceInput) (domain.OfficialSource, error) {
	a, err := requireRole(ctx, auth.RoleEditor)
	if err != nil {
		return domain.OfficialSource{}, err
	}
	id, err := newID()
	if err != nil {
		return domain.OfficialSource{}, err
	}
	s, err := domain.NewOfficialSource(id, in.Domain, in.Name, in.Sphere, a.UserID, uc.clock.Now())
	if err != nil {
		return domain.OfficialSource{}, err
	}
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		return insertOfficialSource(ctx, uc.sources, uc.audit, s, nil)
	})
	if err != nil {
		return domain.OfficialSource{}, err
	}
	return s, nil
}

// insertOfficialSource grava a Fonte oficial e audita, na transação do
// contexto. Domínio já coberto é recusado. extra entra nos metadados (o
// aceite de um Pedido de fonte oficial diz de qual Contribuição veio).
func insertOfficialSource(ctx context.Context, sources OfficialSourceRepo, audit AuditRecorder, s domain.OfficialSource, extra map[string]any) error {
	ds, err := sources.Domains(ctx)
	if err != nil {
		return err
	}
	if domain.IsOfficial(s.Domain, ds) {
		return domain.ErrOfficialSourceExists
	}
	if err := sources.Insert(ctx, s); err != nil {
		return err
	}
	md := map[string]any{"domain": s.Domain, "name": s.Name, "sphere": string(s.Sphere)}
	for k, v := range extra {
		md[k] = v
	}
	return audit.Record(ctx, AuditEvent{
		ActorID: s.CreatedBy, Action: ActionOfficialSourceCreate, TargetType: targetOfficialSource, TargetID: s.ID,
		Metadata: md,
	})
}

// DeleteOfficialSource: só o Admin remove (CONTEXT.md).
type DeleteOfficialSource struct {
	sources OfficialSourceRepo
	tx      TxRunner
	audit   AuditRecorder
}

func NewDeleteOfficialSource(s OfficialSourceRepo, tx TxRunner, a AuditRecorder) *DeleteOfficialSource {
	return &DeleteOfficialSource{sources: s, tx: tx, audit: a}
}

func (uc *DeleteOfficialSource) Execute(ctx context.Context, id string) error {
	a, err := requireRole(ctx, auth.RoleAdmin)
	if err != nil {
		return err
	}
	return uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		s, err := uc.sources.Delete(ctx, id)
		if err != nil {
			return err
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: a.UserID, Action: ActionOfficialSourceDelete, TargetType: targetOfficialSource, TargetID: s.ID,
			Metadata: map[string]any{"domain": s.Domain},
		})
	})
}
