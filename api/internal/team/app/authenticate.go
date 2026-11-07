package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type AuthenticateInput struct {
	AccessToken string
}

// Authenticate valida o token de acesso e devolve o Ator. O token só prova
// quem é a pessoa; a role e o corte de sessão vêm do cadastro a cada
// requisição, então excluir, rebaixar ou resetar a senha de alguém vale na
// hora, e não quando o token expirar.
type Authenticate struct {
	access AccessTokens
	users  UserRepo
}

func NewAuthenticate(access AccessTokens, users UserRepo) *Authenticate {
	return &Authenticate{access: access, users: users}
}

func (uc *Authenticate) Execute(ctx context.Context, in AuthenticateInput) (auth.Actor, error) {
	claims, err := uc.access.Verify(in.AccessToken)
	if err != nil {
		return auth.Actor{}, ErrUnauthenticated
	}
	user, err := uc.users.FindByID(ctx, claims.UserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return auth.Actor{}, ErrUnauthenticated
	}
	if err != nil {
		return auth.Actor{}, fmt.Errorf("buscando usuário: %w", err)
	}
	if !user.SessionValid(claims.IssuedAt) {
		return auth.Actor{}, ErrSessionExpired
	}
	// TODO(#11): recusar quem tem MustChangePassword fora da troca de senha.
	return auth.Actor{UserID: user.ID, Role: auth.Role(user.Role)}, nil
}
