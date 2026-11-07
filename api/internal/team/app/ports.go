// Package app tem os casos de uso do cadastro da equipe e das sessões, e as
// portas de que eles precisam. A autorização acontece aqui, com o Ator
// vindo do contexto (platform/auth); o handler só recusa cedo.
package app

import (
	"context"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

// UserRepo persiste usuários. Erros de domínio: ErrUserNotFound,
// ErrEmailTaken (e-mail já usado, sem diferenciar maiúsculas) e
// ErrCredentialsChanged.
type UserRepo interface {
	// LockUsers serializa, até o fim da transação do contexto, o bootstrap
	// e as mudanças de quem é Admin.
	LockUsers(ctx context.Context) error
	Count(ctx context.Context) (int, error)
	CountAdmins(ctx context.Context) (int, error)
	Create(ctx context.Context, u domain.User) error
	FindByID(ctx context.Context, id string) (domain.User, error)
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	List(ctx context.Context) ([]domain.User, error)
	// UpdatePassword grava hash, MustChangePassword e SessionsValidAfter de
	// u, só se o hash guardado ainda for prevHash.
	UpdatePassword(ctx context.Context, u domain.User, prevHash string) error
	UpdateRole(ctx context.Context, id string, role domain.Role) (domain.User, error)
	Delete(ctx context.Context, id string) error
}

// RefreshTokenRepo persiste sessões. FindByHash devolve
// ErrRefreshTokenInvalid quando não acha.
type RefreshTokenRepo interface {
	Create(ctx context.Context, t domain.RefreshToken) error
	FindByHash(ctx context.Context, hash string) (domain.RefreshToken, error)
	// Revoke consome o token: ErrRefreshTokenReused se outro já o consumiu,
	// ErrRefreshTokenInvalid se ele não existe mais.
	Revoke(ctx context.Context, id string, at time.Time) error
	Delete(ctx context.Context, id string) error
	DeleteAllForUser(ctx context.Context, userID string) error
}

// PasswordHasher isola o algoritmo (bcrypt em produção).
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Verify(plain, hash string) bool
}

// AccessTokens assina e confere o token de acesso (platform/auth.Tokens).
type AccessTokens interface {
	Issue(userID string, role auth.Role) (string, time.Time, error)
	Verify(token string) (auth.Claims, error)
}

// TxRunner abre uma transação levada no contexto: tudo o que os stores
// fizerem com esse contexto (inclusive a Auditoria) é gravado junto ou
// desfeito junto.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// AuditEvent é o que o team manda para a Auditoria. Nunca leva senha nem
// token.
type AuditEvent struct {
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
}

// AuditRecorder grava um evento de Auditoria na transação do contexto. O
// main liga a implementação do módulo audit.
type AuditRecorder interface {
	Record(ctx context.Context, e AuditEvent) error
}

// Ações auditadas do cadastro.
const (
	ActionUserBootstrap     = "user.bootstrap"
	ActionUserCreate        = "user.create"
	ActionUserPasswordReset = "user.password_reset"
	ActionUserRoleChange    = "user.role_change"
	ActionUserDelete        = "user.delete"

	targetUser = "user"
)
