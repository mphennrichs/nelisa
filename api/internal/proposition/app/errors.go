package app

import (
	"context"
	"errors"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
)

var (
	// ErrUnauthenticated: sem Ator no contexto (Leitor anônimo).
	ErrUnauthenticated = errors.New("proposition: não autenticado")
	// ErrForbidden: o Ator não tem a role que a ação exige.
	ErrForbidden = errors.New("proposition: sem permissão")
)

// requireEditor garante que o Ator é da equipe com role Editor ou acima.
func requireEditor(ctx context.Context) (auth.Actor, error) {
	a, ok := auth.ActorFrom(ctx)
	if !ok {
		return auth.Actor{}, ErrUnauthenticated
	}
	if !a.Role.AtLeast(auth.RoleEditor) {
		return a, ErrForbidden
	}
	return a, nil
}
