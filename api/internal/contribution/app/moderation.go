package app

import (
	"context"
	"errors"

	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

// Moderação pela Fila (#25): Editor ou acima decide. Não há quatro olhos,
// porque a Contribuição não publica conteúdo editorial (a Tramitação não
// altera a Revisão); cada decisão é auditada na mesma transação.

// ErrPropositionGone: a Proposição da Atualização (ou a que se quer
// apontar numa Indicação) não existe mais.
var ErrPropositionGone = errors.New("contribution: proposição não encontrada")

// ModerationDeps são as portas da moderação.
type ModerationDeps struct {
	Contributions ContributionRepo
	Sources       OfficialSourceRepo
	Props         Propositions
	Progress      ProgressEntries
	Tx            TxRunner
	Audit         AuditRecorder
	Clock         clock.Clock
}

// ContributionView é a Contribuição como a equipe a vê.
type ContributionView struct {
	domain.Contribution
	// Proposition: a Proposição da Atualização de tramitação.
	Proposition *TeamProposition
	// CreatedProposition: a Proposição criada a partir da Indicação.
	CreatedProposition *TeamProposition
}

// GetContribution: a equipe abre uma Contribuição.
type GetContribution struct{ d ModerationDeps }

func NewGetContribution(d ModerationDeps) *GetContribution { return &GetContribution{d: d} }

func (uc *GetContribution) Execute(ctx context.Context, id string) (ContributionView, error) {
	if _, err := requireRole(ctx, auth.RoleEditor); err != nil {
		return ContributionView{}, err
	}
	c, err := uc.d.Contributions.Find(ctx, id)
	if err != nil {
		return ContributionView{}, err
	}
	return view(ctx, uc.d.Props, c)
}

func view(ctx context.Context, props Propositions, c domain.Contribution) (ContributionView, error) {
	v := ContributionView{Contribution: c}
	var err error
	if c.PropositionID != "" {
		if v.Proposition, err = props.ByID(ctx, c.PropositionID); err != nil {
			return ContributionView{}, err
		}
	}
	if c.CreatedPropositionID != "" {
		if v.CreatedProposition, err = props.ByID(ctx, c.CreatedPropositionID); err != nil {
			return ContributionView{}, err
		}
	}
	return v, nil
}

// ListPending lista as Contribuições pendentes para a Fila. Operação de
// sistema: a Fila (módulo queue) já autorizou quem pede.
type ListPending struct{ contributions ContributionRepo }

func NewListPending(c ContributionRepo) *ListPending { return &ListPending{contributions: c} }

func (uc *ListPending) Execute(ctx context.Context) ([]domain.Contribution, error) {
	return uc.contributions.ListPending(ctx)
}

// moderate abre a transação, trava a Contribuição, aplica fn e grava.
func moderate(ctx context.Context, d ModerationDeps, id string, fn func(ctx context.Context, a auth.Actor, c *domain.Contribution) (AuditEvent, error)) (ContributionView, error) {
	a, err := requireRole(ctx, auth.RoleEditor)
	if err != nil {
		return ContributionView{}, err
	}
	var out domain.Contribution
	err = d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := d.Contributions.FindForUpdate(ctx, id)
		if err != nil {
			return err
		}
		ev, err := fn(ctx, a, &c)
		if err != nil {
			return err
		}
		if err := d.Contributions.Update(ctx, c); err != nil {
			return err
		}
		ev.ActorID, ev.TargetType, ev.TargetID = a.UserID, targetContribution, c.ID
		if ev.Metadata == nil {
			ev.Metadata = map[string]any{}
		}
		ev.Metadata["kind"] = string(c.Kind)
		out = c
		return d.Audit.Record(ctx, ev)
	})
	if err != nil {
		return ContributionView{}, err
	}
	return view(ctx, d.Props, out)
}

// RejectInput: motivo público da lista fechada e nota interna opcional.
type RejectInput struct {
	ID           string
	Reason       domain.RejectReason
	InternalNote string
}

// RejectContribution rejeita uma Contribuição pendente.
type RejectContribution struct{ d ModerationDeps }

func NewRejectContribution(d ModerationDeps) *RejectContribution { return &RejectContribution{d: d} }

func (uc *RejectContribution) Execute(ctx context.Context, in RejectInput) (ContributionView, error) {
	return moderate(ctx, uc.d, in.ID, func(_ context.Context, a auth.Actor, c *domain.Contribution) (AuditEvent, error) {
		if err := c.Reject(a.UserID, in.Reason, in.InternalNote, uc.d.Clock.Now()); err != nil {
			return AuditEvent{}, err
		}
		return AuditEvent{Action: ActionContributionReject, Metadata: map[string]any{"reason": string(in.Reason)}}, nil
	})
}

// ProgressEventInput é o evento manual revisado pela equipe.
type ProgressEventInput struct {
	Chamber     string
	Date        string
	Description string
	SourceURL   string
}

// AcceptInput: nota interna opcional e, na Atualização de tramitação, o
// evento revisado.
type AcceptInput struct {
	ID           string
	InternalNote string
	Event        *ProgressEventInput
}

// AcceptContribution aceita uma Contribuição pendente. Conforme o tipo:
//   - Atualização de tramitação: grava o evento manual revisado na
//     Tramitação da Proposição;
//   - Pedido de fonte oficial: cadastra a Fonte oficial;
//   - Indicação: só marca aceita; a equipe importa a Proposição e depois
//     aponta a criada (LinkProposition).
type AcceptContribution struct{ d ModerationDeps }

func NewAcceptContribution(d ModerationDeps) *AcceptContribution { return &AcceptContribution{d: d} }

// ErrEventRequired: a Atualização de tramitação só é aceita com o evento
// revisado.
var ErrEventRequired = errors.New("contribution: revise o evento antes de aceitar")

func (uc *AcceptContribution) Execute(ctx context.Context, in AcceptInput) (ContributionView, error) {
	return moderate(ctx, uc.d, in.ID, func(ctx context.Context, a auth.Actor, c *domain.Contribution) (AuditEvent, error) {
		now := uc.d.Clock.Now()
		if err := c.Accept(a.UserID, in.InternalNote, now); err != nil {
			return AuditEvent{}, err
		}
		md := map[string]any{}
		switch c.Kind {
		case domain.KindProgressUpdate:
			if in.Event == nil {
				return AuditEvent{}, ErrEventRequired
			}
			ev, err := domain.NewProgressEvent(in.Event.Chamber, in.Event.Date, in.Event.Description, in.Event.SourceURL, now)
			if err != nil {
				return AuditEvent{}, err
			}
			p, err := uc.d.Props.ByID(ctx, c.PropositionID)
			if err != nil {
				return AuditEvent{}, err
			}
			if p == nil {
				return AuditEvent{}, ErrPropositionGone
			}
			if err := uc.d.Progress.RecordManual(ctx, ManualEvent{PropositionID: c.PropositionID, ContributionID: c.ID, ProgressEvent: ev}); err != nil {
				return AuditEvent{}, err
			}
			md["proposition_id"], md["chamber"], md["at"] = c.PropositionID, ev.Chamber, ev.At
		case domain.KindSourceRequest:
			id, err := newID()
			if err != nil {
				return AuditEvent{}, err
			}
			s, err := domain.NewOfficialSource(id, c.Payload.Domain, c.Payload.SourceName, c.Sphere, a.UserID, now)
			if err != nil {
				return AuditEvent{}, err
			}
			if err := insertOfficialSource(ctx, uc.d.Sources, uc.d.Audit, s, map[string]any{"contribution_id": c.ID}); err != nil {
				return AuditEvent{}, err
			}
			md["official_source_id"] = s.ID
		}
		return AuditEvent{Action: ActionContributionAccept, Metadata: md}, nil
	})
}

// CorrectSphereInput: a Esfera certa de uma Indicação ou Pedido pendente.
type CorrectSphereInput struct {
	ID     string
	Sphere domain.Sphere
}

// CorrectSphere: a equipe corrige a Esfera que o Leitor escolheu (manda
// na ordem da Fila).
type CorrectSphere struct{ d ModerationDeps }

func NewCorrectSphere(d ModerationDeps) *CorrectSphere { return &CorrectSphere{d: d} }

func (uc *CorrectSphere) Execute(ctx context.Context, in CorrectSphereInput) (ContributionView, error) {
	return moderate(ctx, uc.d, in.ID, func(_ context.Context, _ auth.Actor, c *domain.Contribution) (AuditEvent, error) {
		from := c.Sphere
		if err := c.CorrectSphere(in.Sphere); err != nil {
			return AuditEvent{}, err
		}
		return AuditEvent{Action: ActionContributionSphere, Metadata: map[string]any{"from": string(from), "to": string(in.Sphere)}}, nil
	})
}

// LinkPropositionInput: a Proposição criada a partir da Indicação.
type LinkPropositionInput struct {
	ID            string
	PropositionID string
}

// LinkProposition guarda a Proposição criada a partir de uma Indicação
// aceita; a consulta do Leitor passa a mostrar o link (se for pública).
type LinkProposition struct{ d ModerationDeps }

func NewLinkProposition(d ModerationDeps) *LinkProposition { return &LinkProposition{d: d} }

func (uc *LinkProposition) Execute(ctx context.Context, in LinkPropositionInput) (ContributionView, error) {
	return moderate(ctx, uc.d, in.ID, func(ctx context.Context, _ auth.Actor, c *domain.Contribution) (AuditEvent, error) {
		if err := c.LinkProposition(in.PropositionID); err != nil {
			return AuditEvent{}, err
		}
		p, err := uc.d.Props.ByID(ctx, in.PropositionID)
		if err != nil {
			return AuditEvent{}, err
		}
		if p == nil {
			return AuditEvent{}, ErrPropositionGone
		}
		return AuditEvent{Action: ActionContributionLink, Metadata: map[string]any{"proposition_id": in.PropositionID}}, nil
	})
}

// DeleteProgressEntry: a equipe apaga o evento manual de uma Atualização
// aceita (por exemplo, quando a Sincronização trouxe o oficial). A
// Contribuição continua aceita.
type DeleteProgressEntry struct{ d ModerationDeps }

func NewDeleteProgressEntry(d ModerationDeps) *DeleteProgressEntry { return &DeleteProgressEntry{d: d} }

func (uc *DeleteProgressEntry) Execute(ctx context.Context, id string) error {
	_, err := moderate(ctx, uc.d, id, func(ctx context.Context, _ auth.Actor, c *domain.Contribution) (AuditEvent, error) {
		if c.Kind != domain.KindProgressUpdate || c.State != domain.StateAccepted {
			return AuditEvent{}, domain.ErrNoProgressEntry
		}
		ok, err := uc.d.Progress.DeleteManual(ctx, c.ID)
		if err != nil {
			return AuditEvent{}, err
		}
		if !ok {
			return AuditEvent{}, domain.ErrNoProgressEntry
		}
		return AuditEvent{Action: ActionContributionProgressDelete, Metadata: map[string]any{"proposition_id": c.PropositionID}}, nil
	})
	return err
}
