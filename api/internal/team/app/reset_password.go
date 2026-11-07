package app

import (
	"context"
	"fmt"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type ResetPasswordInput struct {
	UserID      string
	NewPassword string
}

// ResetPassword é o Admin definindo uma senha nova para alguém que esqueceu
// a sua. A senha é temporária e todas as sessões da pessoa acabam.
type ResetPassword struct {
	users  UserRepo
	tokens RefreshTokenRepo
	hasher PasswordHasher
	tx     TxRunner
	audit  AuditRecorder
	clock  clock.Clock
}

func NewResetPassword(users UserRepo, tokens RefreshTokenRepo, hasher PasswordHasher, tx TxRunner, audit AuditRecorder, clk clock.Clock) *ResetPassword {
	return &ResetPassword{users: users, tokens: tokens, hasher: hasher, tx: tx, audit: audit, clock: clk}
}

func (uc *ResetPassword) Execute(ctx context.Context, in ResetPasswordInput) error {
	actor, err := requireUserManager(ctx)
	if err != nil {
		return err
	}
	if err := domain.ValidatePassword(in.NewPassword); err != nil {
		return err
	}
	user, err := uc.users.FindByID(ctx, in.UserID)
	if err != nil {
		return err
	}
	hash, err := uc.hasher.Hash(in.NewPassword)
	if err != nil {
		return fmt.Errorf("gerando hash da senha: %w", err)
	}
	prev := user.PasswordHash
	user.ResetPassword(hash, uc.clock.Now())
	return uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.users.UpdatePassword(ctx, user, prev); err != nil {
			return err
		}
		if err := uc.tokens.DeleteAllForUser(ctx, user.ID); err != nil {
			return err
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionUserPasswordReset, TargetType: targetUser, TargetID: user.ID,
		})
	})
}
