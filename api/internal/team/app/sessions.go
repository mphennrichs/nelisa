package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

// Session é o que login, bootstrap, refresh e troca de senha entregam: o
// token de acesso vai no corpo da resposta e o refresh token no cookie.
type Session struct {
	User             domain.User
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// Sessions emite o par de tokens. Compartilhado pelos casos de uso que
// abrem sessão, para a sequência "gera refresh, guarda o hash, assina o
// acesso" existir num lugar só.
type Sessions struct {
	access AccessTokens
	tokens RefreshTokenRepo
	clock  clock.Clock
}

func NewSessions(access AccessTokens, tokens RefreshTokenRepo, clk clock.Clock) *Sessions {
	return &Sessions{access: access, tokens: tokens, clock: clk}
}

func (s *Sessions) open(ctx context.Context, u domain.User) (Session, error) {
	access, accessExp, err := s.access.Issue(u.ID, auth.Role(u.Role))
	if err != nil {
		return Session{}, fmt.Errorf("assinando token de acesso: %w", err)
	}
	raw, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	id, err := newID()
	if err != nil {
		return Session{}, err
	}
	t := domain.NewRefreshToken(id, u.ID, HashRefreshToken(raw), s.clock.Now())
	if err := s.tokens.Create(ctx, t); err != nil {
		return Session{}, fmt.Errorf("gravando refresh token: %w", err)
	}
	return Session{
		User: u, AccessToken: access, AccessExpiresAt: accessExp,
		RefreshToken: raw, RefreshExpiresAt: t.ExpiresAt,
	}, nil
}

// HashRefreshToken é o que fica no banco: vazando a tabela, nenhum token
// vale.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("gerando refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// newID gera um UUID v4.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("gerando id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
