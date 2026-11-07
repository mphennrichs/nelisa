package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type LogoutInput struct {
	RefreshToken string
}

// Logout apaga a sessão no servidor. Idempotente: token desconhecido conta
// como sucesso.
type Logout struct {
	tokens RefreshTokenRepo
}

func NewLogout(tokens RefreshTokenRepo) *Logout { return &Logout{tokens: tokens} }

func (uc *Logout) Execute(ctx context.Context, in LogoutInput) error {
	if in.RefreshToken == "" {
		return nil
	}
	token, err := uc.tokens.FindByHash(ctx, HashRefreshToken(in.RefreshToken))
	if errors.Is(err, domain.ErrRefreshTokenInvalid) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("buscando refresh token: %w", err)
	}
	// Apagado, não revogado: um token revogado ainda vale durante a
	// carência, e a cópia de um cookie de quem saiu não pode valer.
	if err := uc.tokens.Delete(ctx, token.ID); err != nil {
		return fmt.Errorf("apagando refresh token: %w", err)
	}
	return nil
}
