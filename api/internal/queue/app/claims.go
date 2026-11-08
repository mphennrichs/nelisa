package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/queue/domain"
)

type ClaimInput struct{ PropositionID string }

// Claim é alguém da equipe assumindo a Posse de uma Proposição. Se outra
// pessoa a tem e ela vale, 409 com o nome dela; se já é sua, renova.
type Claim struct {
	claims ClaimRepo
	props  Propositions
	people People
	tx     TxRunner
	clock  clock.Clock
}

func NewClaim(claims ClaimRepo, props Propositions, people People, tx TxRunner, clk clock.Clock) *Claim {
	return &Claim{claims: claims, props: props, people: people, tx: tx, clock: clk}
}

func (uc *Claim) Execute(ctx context.Context, in ClaimInput) (ClaimView, error) {
	a, err := actor(ctx)
	if err != nil {
		return ClaimView{}, err
	}
	ok, err := uc.props.Exists(ctx, in.PropositionID)
	if err != nil {
		return ClaimView{}, err
	}
	if !ok {
		return ClaimView{}, ErrPropositionNotFound
	}
	var out ClaimView
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		c, err := take(ctx, uc.claims, uc.people, in.PropositionID, a.UserID, uc.clock)
		if err != nil {
			return err
		}
		ps, err := uc.people.Lookup(ctx, []string{a.UserID})
		if err != nil {
			return err
		}
		out = ClaimView{Claim: c, HolderName: ps[a.UserID].Name}
		return nil
	})
	if err != nil {
		return ClaimView{}, err
	}
	return out, nil
}

// take assume ou renova a Posse dentro da transação do contexto.
func take(ctx context.Context, claims ClaimRepo, people People, propositionID, userID string, clk clock.Clock) (domain.Claim, error) {
	if err := claims.Lock(ctx, propositionID); err != nil {
		return domain.Claim{}, err
	}
	now := clk.Now()
	existing, valid, err := current(ctx, claims, people, propositionID, now)
	if err != nil {
		return domain.Claim{}, err
	}
	c, err := domain.Take(existing, valid != nil, propositionID, userID, now)
	if err != nil {
		if valid != nil {
			return domain.Claim{}, &domain.ClaimTakenError{HolderID: valid.UserID, HolderName: valid.HolderName}
		}
		return domain.Claim{}, err
	}
	if err := claims.Save(ctx, c); err != nil {
		return domain.Claim{}, err
	}
	return c, nil
}

type ReleaseInput struct{ PropositionID string }

// Release solta a Posse. Quem a tem solta a própria; Revisor e Admin
// soltam a de qualquer um, e isso fica na Auditoria. Posse que já não vale
// (expirada, conta desativada) é só limpa.
type Release struct {
	claims ClaimRepo
	people People
	tx     TxRunner
	audit  AuditRecorder
	clock  clock.Clock
}

func NewRelease(claims ClaimRepo, people People, tx TxRunner, audit AuditRecorder, clk clock.Clock) *Release {
	return &Release{claims: claims, people: people, tx: tx, audit: audit, clock: clk}
}

func (uc *Release) Execute(ctx context.Context, in ReleaseInput) error {
	a, err := actor(ctx)
	if err != nil {
		return err
	}
	return uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.claims.Lock(ctx, in.PropositionID); err != nil {
			return err
		}
		existing, valid, err := current(ctx, uc.claims, uc.people, in.PropositionID, uc.clock.Now())
		if err != nil || existing == nil {
			return err
		}
		othersValid := valid != nil && valid.UserID != a.UserID
		if valid != nil && !domain.CanRelease(valid.UserID, a.UserID, a.Role.AtLeast(auth.RoleReviewer)) {
			return domain.ErrNotHolder
		}
		if err := uc.claims.Delete(ctx, in.PropositionID); err != nil {
			return err
		}
		if !othersValid {
			return nil
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: a.UserID, Action: ActionClaimRelease, TargetType: targetProposition, TargetID: in.PropositionID,
			Metadata: map[string]any{"holder_id": valid.UserID, "holder_name": valid.HolderName},
		})
	})
}

// Gate são as operações de Posse que os outros módulos usam, ligadas no
// main. Não autorizam nem abrem transação: quem chama já fez isso.
type Gate struct {
	claims ClaimRepo
	people People
	clock  clock.Clock
}

func NewGate(claims ClaimRepo, people People, clk clock.Clock) *Gate {
	return &Gate{claims: claims, people: people, clock: clk}
}

// TouchForEdit é userID gravando conteúdo: assume a Posse se livre,
// renova a atividade se já é dele, *ClaimTakenError se é de outra pessoa.
func (g *Gate) TouchForEdit(ctx context.Context, propositionID, userID string) error {
	_, err := take(ctx, g.claims, g.people, propositionID, userID, g.clock)
	return err
}

// Current devolve a Posse que vale agora, ou nil se está livre.
func (g *Gate) Current(ctx context.Context, propositionID string) (*ClaimView, error) {
	_, v, err := current(ctx, g.claims, g.people, propositionID, g.clock.Now())
	return v, err
}

// Release solta a Posse (a aprovação faz isso).
func (g *Gate) Release(ctx context.Context, propositionID string) error {
	if err := g.claims.Lock(ctx, propositionID); err != nil {
		return err
	}
	return g.claims.Delete(ctx, propositionID)
}

// CanReleaseFor diz se o Ator do contexto pode soltar a Posse de holderID.
// O detalhe da Proposição usa isso para o front não repetir a regra.
func CanReleaseFor(ctx context.Context, holderID string) bool {
	a, err := actor(ctx)
	if err != nil {
		return false
	}
	return domain.CanRelease(holderID, a.UserID, a.Role.AtLeast(auth.RoleReviewer))
}
