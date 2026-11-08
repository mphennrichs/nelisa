package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type LoginInput struct {
	Email    string
	Password string
}

// Login troca e-mail e senha por uma sessão.
//
// TODO(#13): registrar login (sucesso e falha) na Auditoria.
type Login struct {
	users    UserRepo
	hasher   PasswordHasher
	sessions *Sessions

	dummyOnce sync.Once
	dummy     string
}

func NewLogin(users UserRepo, hasher PasswordHasher, sessions *Sessions) *Login {
	return &Login{users: users, hasher: hasher, sessions: sessions}
}

// dummyHash é conferido quando o e-mail não existe, para que a resposta
// demore o mesmo que uma senha errada e o tempo não revele quem tem conta.
func (uc *Login) dummyHash() string {
	uc.dummyOnce.Do(func() { uc.dummy, _ = uc.hasher.Hash("nelisa-usuario-inexistente") })
	return uc.dummy
}

func (uc *Login) Execute(ctx context.Context, in LoginInput) (Session, error) {
	user, err := uc.users.FindByEmail(ctx, in.Email)
	if errors.Is(err, domain.ErrUserNotFound) {
		uc.hasher.Verify(in.Password, uc.dummyHash())
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, fmt.Errorf("buscando usuário: %w", err)
	}
	if !uc.hasher.Verify(in.Password, user.PasswordHash) {
		return Session{}, ErrInvalidCredentials
	}
	if !user.Active() {
		return Session{}, ErrAccountDeactivated
	}
	return uc.sessions.open(ctx, user)
}
