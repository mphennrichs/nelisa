package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

type LoginInput struct {
	Email    string
	Password string
}

// Login troca e-mail e senha por uma sessão. Todo login vai para a
// Auditoria (#13): o que deu certo, junto com a sessão; o que falhou, com o
// motivo e sem ator. Nem o e-mail digitado nem o IP são gravados: um e-mail
// sem conta vira um evento sem alvo identificável, e gravá-lo mesmo assim
// mantém o tempo de resposta igual ao de uma conta que existe.
type Login struct {
	users    UserRepo
	hasher   PasswordHasher
	sessions *Sessions
	tx       TxRunner
	audit    AuditRecorder

	dummyOnce sync.Once
	dummy     string
}

func NewLogin(users UserRepo, hasher PasswordHasher, sessions *Sessions, tx TxRunner, audit AuditRecorder) *Login {
	return &Login{users: users, hasher: hasher, sessions: sessions, tx: tx, audit: audit}
}

// Motivos de falha de login gravados na Auditoria.
const (
	LoginFailedUnknownEmail = "unknown_email"
	LoginFailedPassword     = "invalid_password"
	LoginFailedDeactivated  = "deactivated"
)

// failed grava a falha e devolve errOut, ou o erro da Auditoria: sem o
// registro, a resposta não sai.
func (uc *Login) failed(ctx context.Context, userID, reason string, errOut error) error {
	target, id := targetUser, userID
	if userID == "" {
		target, id = targetLogin, LoginFailedUnknownEmail
	}
	err := uc.audit.Record(ctx, AuditEvent{
		Action: ActionUserLoginFailed, TargetType: target, TargetID: id,
		Metadata: map[string]any{"reason": reason},
	})
	if err != nil {
		return fmt.Errorf("auditando falha de login: %w", err)
	}
	return errOut
}

// dummyHash é conferido quando o e-mail não existe, para que a resposta
// demore o mesmo que uma senha errada e o tempo não revele quem tem conta.
func (uc *Login) dummyHash() string {
	uc.dummyOnce.Do(func() { uc.dummy, _ = uc.hasher.Hash("nelisa-usuario-inexistente") })
	return uc.dummy
}

func (uc *Login) Execute(ctx context.Context, in LoginInput) (Session, error) {
	user, err := uc.users.FindByEmail(ctx, in.Email)
	if errors.Is(err, domain.ErrUserNotFound) {
		uc.hasher.Verify(in.Password, uc.dummyHash())
		return Session{}, uc.failed(ctx, "", LoginFailedUnknownEmail, ErrInvalidCredentials)
	}
	if err != nil {
		return Session{}, fmt.Errorf("buscando usuário: %w", err)
	}
	if !uc.hasher.Verify(in.Password, user.PasswordHash) {
		return Session{}, uc.failed(ctx, user.ID, LoginFailedPassword, ErrInvalidCredentials)
	}
	if !user.Active() {
		return Session{}, uc.failed(ctx, user.ID, LoginFailedDeactivated, ErrAccountDeactivated)
	}
	var s Session
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if s, err = uc.sessions.open(ctx, user); err != nil {
			return err
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: user.ID, Action: ActionUserLogin, TargetType: targetUser, TargetID: user.ID,
		})
	})
	if err != nil {
		return Session{}, err
	}
	return s, nil
}
