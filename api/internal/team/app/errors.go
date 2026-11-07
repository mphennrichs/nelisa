package app

import (
	"context"
	"errors"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

var (
	// ErrUnauthenticated: sem Ator no contexto, ou token de acesso inválido.
	ErrUnauthenticated = errors.New("team: não autenticado")
	// ErrSessionExpired: o token é de antes de uma troca ou reset de senha.
	ErrSessionExpired = errors.New("team: sessão encerrada")
	// ErrForbidden: o Ator não tem a role que a ação exige.
	ErrForbidden = errors.New("team: sem permissão")
	// ErrInvalidCredentials não diz se o e-mail existe.
	ErrInvalidCredentials = errors.New("team: e-mail ou senha incorretos")
	// ErrBootstrapDisabled: BOOTSTRAP_TOKEN não configurado.
	ErrBootstrapDisabled     = errors.New("team: bootstrap desligado")
	ErrBootstrapTokenInvalid = errors.New("team: token de bootstrap incorreto")
	ErrAlreadyBootstrapped   = errors.New("team: já existe usuário cadastrado")
)

// currentActor lê o Ator posto no contexto pelo middleware.
func currentActor(ctx context.Context) (auth.Actor, error) {
	a, ok := auth.ActorFrom(ctx)
	if !ok {
		return auth.Actor{}, ErrUnauthenticated
	}
	return a, nil
}

// requireUserManager garante que o Ator pode administrar a equipe.
func requireUserManager(ctx context.Context) (auth.Actor, error) {
	a, err := currentActor(ctx)
	if err != nil {
		return a, err
	}
	if !domain.CanManageUsers(domain.Role(a.Role)) {
		return a, ErrForbidden
	}
	return a, nil
}
