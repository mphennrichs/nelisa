package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

// AccessTokenTTL é a validade do token de acesso. Curto porque não há como
// revogá-lo um a um: a renovação passa pelo refresh token, que é revogável.
const AccessTokenTTL = 15 * time.Minute

const (
	issuer   = "nelisa"
	audience = "nelisa-api"
)

// ErrInvalidToken cobre token ausente, mal formado, expirado ou com
// assinatura errada. O motivo exato não sai daqui: não ajuda o cliente e
// ajuda quem tenta forjar.
var ErrInvalidToken = errors.New("auth: token de acesso inválido")

// Claims é o que o token de acesso prova.
type Claims struct {
	UserID   string
	Role     Role
	IssuedAt time.Time
}

// jwtClaims leva, além do iat padrão (segundos), o instante de emissão em
// milissegundos inteiros: a sessão é cortada comparando-o com
// sessions_valid_after, e um iat em segundos deixaria passar um token
// emitido instantes antes de um reset de senha (o iat fracionário da
// biblioteca perde precisão no caminho).
type jwtClaims struct {
	Role       string `json:"role"`
	IssuedAtMs int64  `json:"iat_ms"`
	jwt.RegisteredClaims
}

// Tokens assina e confere tokens de acesso HS256.
type Tokens struct {
	secret []byte
	clock  clock.Clock
}

// NewTokens recebe o JWT_SECRET já validado pelo config.
func NewTokens(secret string, clk clock.Clock) Tokens {
	return Tokens{secret: []byte(secret), clock: clk}
}

// Issue assina um token de acesso. Devolve também quando ele expira.
func (t Tokens) Issue(userID string, role Role) (string, time.Time, error) {
	now := t.clock.Now().UTC().Truncate(time.Millisecond)
	exp := now.Add(AccessTokenTTL)
	c := jwtClaims{
		Role:       string(role),
		IssuedAtMs: now.UnixMilli(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("assinando token de acesso: %w", err)
	}
	return s, exp, nil
}

// Verify confere assinatura, algoritmo, emissor, audiência e validade.
func (t Tokens) Verify(token string) (Claims, error) {
	c := &jwtClaims{}
	_, err := jwt.ParseWithClaims(token, c, func(*jwt.Token) (any, error) { return t.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithTimeFunc(t.clock.Now),
	)
	if err != nil || c.IssuedAtMs <= 0 || c.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	return Claims{UserID: c.Subject, Role: Role(c.Role), IssuedAt: time.UnixMilli(c.IssuedAtMs).UTC()}, nil
}
