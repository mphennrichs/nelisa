package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type SetUserActiveInput struct {
	UserID string
}

// DeactivateUser é o Admin tirando o acesso de alguém. A conta não é
// apagada: Auditoria, Autoria e aprovações continuam apontando para ela. O
// login passa a ser recusado e todas as sessões acabam na hora. Ninguém
// desativa a si mesmo nem o último Admin ativo.
type DeactivateUser struct {
	users  UserRepo
	tokens RefreshTokenRepo
	tx     TxRunner
	audit  AuditRecorder
	clock  clock.Clock
}

func NewDeactivateUser(users UserRepo, tokens RefreshTokenRepo, tx TxRunner, audit AuditRecorder, clk clock.Clock) *DeactivateUser {
	return &DeactivateUser{users: users, tokens: tokens, tx: tx, audit: audit, clock: clk}
}

func (uc *DeactivateUser) Execute(ctx context.Context, in SetUserActiveInput) (domain.User, error) {
	actor, err := requireUserManager(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if in.UserID == actor.UserID {
		return domain.User{}, domain.ErrCannotDeactivateSelf
	}
	var out domain.User
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.users.LockUsers(ctx); err != nil {
			return err
		}
		u, err := uc.users.FindByID(ctx, in.UserID)
		if err != nil {
			return err
		}
		out = u
		if !u.Active() {
			return nil
		}
		u.Deactivate(uc.clock.Now())
		if err := uc.users.UpdateDeactivation(ctx, u); err != nil {
			return err
		}
		if err := uc.tokens.DeleteAllForUser(ctx, u.ID); err != nil {
			return err
		}
		if u.Role == domain.RoleAdmin {
			n, err := uc.users.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if n == 0 {
				return domain.ErrLastAdmin
			}
		}
		out = u
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionUserDeactivate, TargetType: targetUser, TargetID: u.ID,
			Metadata: map[string]any{"name": u.Name, "role": string(u.Role)},
		})
	})
	if err != nil {
		return domain.User{}, err
	}
	return out, nil
}

// ReactivateUser devolve o acesso a uma conta desativada. A pessoa entra
// com a senha que tinha; se o Admin quiser outra, usa o reset de senha.
type ReactivateUser struct {
	users UserRepo
	tx    TxRunner
	audit AuditRecorder
}

func NewReactivateUser(users UserRepo, tx TxRunner, audit AuditRecorder) *ReactivateUser {
	return &ReactivateUser{users: users, tx: tx, audit: audit}
}

func (uc *ReactivateUser) Execute(ctx context.Context, in SetUserActiveInput) (domain.User, error) {
	actor, err := requireUserManager(ctx)
	if err != nil {
		return domain.User{}, err
	}
	var out domain.User
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.users.LockUsers(ctx); err != nil {
			return err
		}
		u, err := uc.users.FindByID(ctx, in.UserID)
		if err != nil {
			return err
		}
		out = u
		if u.Active() {
			return nil
		}
		u.Reactivate()
		if err := uc.users.UpdateDeactivation(ctx, u); err != nil {
			return err
		}
		out = u
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionUserReactivate, TargetType: targetUser, TargetID: u.ID,
			Metadata: map[string]any{"name": u.Name, "role": string(u.Role)},
		})
	})
	if err != nil {
		return domain.User{}, err
	}
	return out, nil
}
