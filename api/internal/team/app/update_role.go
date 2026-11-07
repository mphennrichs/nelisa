package app

import (
	"context"

	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type UpdateRoleInput struct {
	UserID string
	Role   string
}

// UpdateRole é o Admin trocando a role de alguém. Nunca deixa o Nelisa sem
// Admin. Vale na próxima requisição da pessoa: o middleware lê a role do
// cadastro, não do token.
type UpdateRole struct {
	users UserRepo
	tx    TxRunner
	audit AuditRecorder
}

func NewUpdateRole(users UserRepo, tx TxRunner, audit AuditRecorder) *UpdateRole {
	return &UpdateRole{users: users, tx: tx, audit: audit}
}

func (uc *UpdateRole) Execute(ctx context.Context, in UpdateRoleInput) (domain.User, error) {
	actor, err := requireUserManager(ctx)
	if err != nil {
		return domain.User{}, err
	}
	role, err := domain.ParseRole(in.Role)
	if err != nil {
		return domain.User{}, err
	}
	var out domain.User
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.users.LockUsers(ctx); err != nil {
			return err
		}
		before, err := uc.users.FindByID(ctx, in.UserID)
		if err != nil {
			return err
		}
		if before.Role == role {
			out = before
			return nil
		}
		if out, err = uc.users.UpdateRole(ctx, in.UserID, role); err != nil {
			return err
		}
		if before.Role == domain.RoleAdmin {
			n, err := uc.users.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if n == 0 {
				return domain.ErrLastAdmin
			}
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionUserRoleChange, TargetType: targetUser, TargetID: in.UserID,
			Metadata: map[string]any{"from": string(before.Role), "to": string(role)},
		})
	})
	if err != nil {
		return domain.User{}, err
	}
	return out, nil
}
