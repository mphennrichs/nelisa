package app

import (
	"context"
	"crypto/subtle"
	"fmt"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type BootstrapInput struct {
	Token    string
	Name     string
	Email    string
	Password string
}

// Bootstrap cria a primeira pessoa, que é Admin. É o único jeito de criar
// conta sem um Admin, e só funciona com BOOTSTRAP_TOKEN configurado e
// enquanto não houver nenhum usuário: a rota é pública, e sem o token quem
// chegasse primeiro a um deploy novo viraria Admin.
type Bootstrap struct {
	users    UserRepo
	hasher   PasswordHasher
	sessions *Sessions
	tx       TxRunner
	audit    AuditRecorder
	clock    clock.Clock
	secret   string
}

func NewBootstrap(users UserRepo, hasher PasswordHasher, sessions *Sessions, tx TxRunner, audit AuditRecorder, clk clock.Clock, bootstrapToken string) *Bootstrap {
	return &Bootstrap{users: users, hasher: hasher, sessions: sessions, tx: tx, audit: audit, clock: clk, secret: bootstrapToken}
}

func (uc *Bootstrap) Execute(ctx context.Context, in BootstrapInput) (Session, error) {
	if uc.secret == "" {
		return Session{}, ErrBootstrapDisabled
	}
	// O token é conferido antes de qualquer consulta: sem ele, ninguém
	// descobre nem se ainda há bootstrap a fazer.
	if subtle.ConstantTimeCompare([]byte(in.Token), []byte(uc.secret)) != 1 {
		return Session{}, ErrBootstrapTokenInvalid
	}
	if err := domain.ValidatePassword(in.Password); err != nil {
		return Session{}, err
	}
	id, err := newID()
	if err != nil {
		return Session{}, err
	}
	// Valida antes do bcrypt, que é lento de propósito.
	if _, err := domain.NewUser(id, in.Name, in.Email, "-", domain.RoleAdmin, false, uc.clock.Now()); err != nil {
		return Session{}, err
	}
	hash, err := uc.hasher.Hash(in.Password)
	if err != nil {
		return Session{}, fmt.Errorf("gerando hash da senha: %w", err)
	}
	user, err := domain.NewUser(id, in.Name, in.Email, hash, domain.RoleAdmin, false, uc.clock.Now())
	if err != nil {
		return Session{}, err
	}
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Trava e conta na mesma transação: dois bootstraps simultâneos
		// não criam dois Admins.
		if err := uc.users.LockUsers(ctx); err != nil {
			return err
		}
		n, err := uc.users.Count(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrAlreadyBootstrapped
		}
		if err := uc.users.Create(ctx, user); err != nil {
			return err
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: user.ID, Action: ActionUserBootstrap, TargetType: targetUser, TargetID: user.ID,
			Metadata: map[string]any{"role": string(user.Role)},
		})
	})
	if err != nil {
		return Session{}, err
	}
	return uc.sessions.open(ctx, user)
}
