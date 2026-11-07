package domain

import (
	"errors"
	"time"
)

var (
	ErrRefreshTokenInvalid = errors.New("team: refresh token inválido")
	ErrRefreshTokenExpired = errors.New("team: refresh token expirado")
	// ErrRefreshTokenReused: um token já gasto voltou depois da carência;
	// alguém guardou uma cópia, e todas as sessões da pessoa acabam.
	ErrRefreshTokenReused = errors.New("team: refresh token já usado")
)

// RefreshTokenTTL é a validade de uma sessão sem uso.
const RefreshTokenTTL = 30 * 24 * time.Hour

// ReuseGrace é quanto tempo um token recém-gasto ainda pode ser trocado: o
// navegador costuma renovar várias vezes ao mesmo tempo (abas, requisições
// paralelas que levaram 401) com o mesmo cookie.
const ReuseGrace = 30 * time.Second

// RefreshToken é a sessão: um valor opaco e aleatório cujo hash fica no
// banco. Gira a cada uso (o apresentado é revogado e outro é emitido), então
// uma cópia roubada serve no máximo uma vez antes de ser descoberta.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

func NewRefreshToken(id, userID, tokenHash string, now time.Time) RefreshToken {
	return RefreshToken{ID: id, UserID: userID, TokenHash: tokenHash, CreatedAt: now, ExpiresAt: now.Add(RefreshTokenTTL)}
}

func (t RefreshToken) IsExpired(now time.Time) bool { return !now.Before(t.ExpiresAt) }

// ReusedAfterGrace: o token já foi gasto e voltou depois da carência.
func (t RefreshToken) ReusedAfterGrace(now time.Time) bool {
	return t.RevokedAt != nil && now.Sub(*t.RevokedAt) >= ReuseGrace
}
