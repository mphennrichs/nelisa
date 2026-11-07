package app

import (
	"context"
	"fmt"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type CreateUserInput struct {
	Name     string
	Email    string
	Password string
	Role     string
}

// CreateUser é o Admin cadastrando uma pessoa (ADR 0001: sem convite nem
// e-mail na v1). A senha definida pelo Admin é temporária.
type CreateUser struct {
	users  UserRepo
	hasher PasswordHasher
	tx     TxRunner
	audit  AuditRecorder
	clock  clock.Clock
}

func NewCreateUser(users UserRepo, hasher PasswordHasher, tx TxRunner, audit AuditRecorder, clk clock.Clock) *CreateUser {
	return &CreateUser{users: users, hasher: hasher, tx: tx, audit: audit, clock: clk}
}

func (uc *CreateUser) Execute(ctx context.Context, in CreateUserInput) (domain.User, error) {
	actor, err := requireUserManager(ctx)
	if err != nil {
		return domain.User{}, err
	}
	role, err := domain.ParseRole(in.Role)
	if err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidatePassword(in.Password); err != nil {
		return domain.User{}, err
	}
	id, err := newID()
	if err != nil {
		return domain.User{}, err
	}
	if _, err := domain.NewUser(id, in.Name, in.Email, "-", role, true, uc.clock.Now()); err != nil {
		return domain.User{}, err
	}
	hash, err := uc.hasher.Hash(in.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("gerando hash da senha: %w", err)
	}
	user, err := domain.NewUser(id, in.Name, in.Email, hash, role, true, uc.clock.Now())
	if err != nil {
		return domain.User{}, err
	}
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.users.Create(ctx, user); err != nil {
			return err
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionUserCreate, TargetType: targetUser, TargetID: user.ID,
			Metadata: map[string]any{"name": user.Name, "role": string(user.Role)},
		})
	})
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}
