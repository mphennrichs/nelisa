package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type DeleteUserInput struct {
	UserID string
}

// DeleteUser é o Admin excluindo alguém (como no identity de origem: a conta
// some, e as sessões vão junto). A Auditoria guarda o histórico, que não
// depende da conta existir. Ninguém exclui a si mesmo nem o último Admin.
type DeleteUser struct {
	users UserRepo
	tx    TxRunner
	audit AuditRecorder
}

func NewDeleteUser(users UserRepo, tx TxRunner, audit AuditRecorder) *DeleteUser {
	return &DeleteUser{users: users, tx: tx, audit: audit}
}

func (uc *DeleteUser) Execute(ctx context.Context, in DeleteUserInput) error {
	actor, err := requireUserManager(ctx)
	if err != nil {
		return err
	}
	if in.UserID == actor.UserID {
		return domain.ErrCannotDeleteSelf
	}
	return uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.users.LockUsers(ctx); err != nil {
			return err
		}
		target, err := uc.users.FindByID(ctx, in.UserID)
		if err != nil {
			return err
		}
		if err := uc.users.Delete(ctx, in.UserID); err != nil {
			return err
		}
		if target.Role == domain.RoleAdmin {
			n, err := uc.users.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if n == 0 {
				return domain.ErrLastAdmin
			}
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionUserDelete, TargetType: targetUser, TargetID: in.UserID,
			Metadata: map[string]any{"name": target.Name, "role": string(target.Role)},
		})
	})
}
