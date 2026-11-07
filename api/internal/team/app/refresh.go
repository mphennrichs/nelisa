package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type RefreshInput struct {
	RefreshToken string
}

// Refresh troca um refresh token por uma sessão nova, girando-o: o token
// apresentado é revogado e outro é emitido. A role é relida do cadastro, e
// não do token antigo.
//
// Várias renovações simultâneas com o mesmo cookie (abas, requisições
// paralelas) são normais: só uma consome o token, e as outras, dentro de
// domain.ReuseGrace, também recebem sessão. Um token gasto que volta depois
// disso foi copiado: todas as sessões da pessoa acabam.
//
// Não roda numa transação de propósito: a exclusão das sessões ao detectar
// reuso precisa ficar gravada mesmo com o erro devolvido.
type Refresh struct {
	tokens   RefreshTokenRepo
	users    UserRepo
	sessions *Sessions
	clock    clock.Clock
}

func NewRefresh(tokens RefreshTokenRepo, users UserRepo, sessions *Sessions, clk clock.Clock) *Refresh {
	return &Refresh{tokens: tokens, users: users, sessions: sessions, clock: clk}
}

func (uc *Refresh) Execute(ctx context.Context, in RefreshInput) (Session, error) {
	if in.RefreshToken == "" {
		return Session{}, domain.ErrRefreshTokenInvalid
	}
	token, err := uc.tokens.FindByHash(ctx, HashRefreshToken(in.RefreshToken))
	if err != nil {
		return Session{}, err
	}
	now := uc.clock.Now()
	if token.IsExpired(now) {
		return Session{}, domain.ErrRefreshTokenExpired
	}
	if token.ReusedAfterGrace(now) {
		return Session{}, uc.endAllSessions(ctx, token.UserID)
	}
	if token.RevokedAt == nil {
		// Revoke é o "consumir" atômico; perder a corrida para outra
		// renovação com o mesmo token é o caso paralelo, dentro da carência.
		err := uc.tokens.Revoke(ctx, token.ID, now)
		switch {
		case errors.Is(err, domain.ErrRefreshTokenInvalid):
			return Session{}, err // apagado nesse meio-tempo: logout ou reset
		case err != nil && !errors.Is(err, domain.ErrRefreshTokenReused):
			return Session{}, fmt.Errorf("revogando refresh token: %w", err)
		}
	}
	user, err := uc.users.FindByID(ctx, token.UserID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return Session{}, domain.ErrRefreshTokenInvalid
	}
	if err != nil {
		return Session{}, fmt.Errorf("buscando usuário: %w", err)
	}
	// Um token de antes de uma troca ou reset de senha está morto mesmo
	// que uma renovação concorrente o tenha lido antes da limpeza.
	if !user.SessionValid(token.CreatedAt) {
		return Session{}, domain.ErrRefreshTokenInvalid
	}
	return uc.sessions.open(ctx, user)
}

func (uc *Refresh) endAllSessions(ctx context.Context, userID string) error {
	if err := uc.tokens.DeleteAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("encerrando sessões após reuso de token: %w", err)
	}
	return domain.ErrRefreshTokenReused
}
