package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type ChangePasswordInput struct {
	CurrentPassword string
	NewPassword     string
}

// ChangePassword é a troca de senha pela própria pessoa. Exige a senha
// atual (uma sessão esquecida aberta não basta para tomar a conta) e
// encerra todas as sessões, inclusive a de quem pediu, que recebe uma nova.
type ChangePassword struct {
	users    UserRepo
	tokens   RefreshTokenRepo
	hasher   PasswordHasher
	sessions *Sessions
	tx       TxRunner
	clock    clock.Clock
}

func NewChangePassword(users UserRepo, tokens RefreshTokenRepo, hasher PasswordHasher, sessions *Sessions, tx TxRunner, clk clock.Clock) *ChangePassword {
	return &ChangePassword{users: users, tokens: tokens, hasher: hasher, sessions: sessions, tx: tx, clock: clk}
}

func (uc *ChangePassword) Execute(ctx context.Context, in ChangePasswordInput) (Session, error) {
	actor, err := currentActor(ctx)
	if err != nil {
		return Session{}, err
	}
	user, err := uc.users.FindByID(ctx, actor.UserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return Session{}, ErrUnauthenticated
	}
	if err != nil {
		return Session{}, fmt.Errorf("buscando usuário: %w", err)
	}
	if !uc.hasher.Verify(in.CurrentPassword, user.PasswordHash) {
		return Session{}, ErrInvalidCredentials
	}
	if err := domain.ValidatePassword(in.NewPassword); err != nil {
		return Session{}, err
	}
	hash, err := uc.hasher.Hash(in.NewPassword)
	if err != nil {
		return Session{}, fmt.Errorf("gerando hash da senha: %w", err)
	}
	prev := user.PasswordHash
	user.ChangePassword(hash, uc.clock.Now())
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := uc.users.UpdatePassword(ctx, user, prev); err != nil {
			return err
		}
		return uc.tokens.DeleteAllForUser(ctx, user.ID)
	})
	if err != nil {
		return Session{}, err
	}
	return uc.sessions.open(ctx, user)
}
