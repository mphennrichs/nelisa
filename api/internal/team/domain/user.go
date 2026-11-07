// Package domain tem as regras do cadastro da equipe (ADR 0001): quem é
// usuário, que role tem, como a senha e as sessões se comportam. Só Go
// padrão.
package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrUserNotFound     = errors.New("team: usuário não encontrado")
	ErrEmailTaken       = errors.New("team: e-mail já cadastrado")
	ErrInvalidEmail     = errors.New("team: e-mail inválido")
	ErrInvalidName      = errors.New("team: nome obrigatório, até 120 caracteres")
	ErrInvalidRole      = errors.New("team: role deve ser editor, reviewer ou admin")
	ErrPasswordTooShort = errors.New("team: senha com menos de 8 caracteres")
	ErrPasswordTooLong  = errors.New("team: senha com mais de 72 bytes")
	// ErrLastAdmin: a mudança deixaria o Nelisa sem nenhum Admin, e só o
	// Admin cadastra pessoas.
	ErrLastAdmin        = errors.New("team: precisa sobrar ao menos um Admin")
	ErrCannotDeleteSelf = errors.New("team: ninguém exclui a própria conta")
	// ErrCredentialsChanged: a senha mudou entre a leitura e a gravação.
	ErrCredentialsChanged = errors.New("team: a senha mudou nesse meio-tempo; tente de novo")
)

// MinPasswordLen é o mínimo da ADR 0001. A checagem contra senhas vazadas
// é a issue #12.
const MinPasswordLen = 8

// maxPasswordBytes: o bcrypt ignora (ou recusa) o que passa de 72 bytes.
const maxPasswordBytes = 72

const maxNameLen = 120

// Role é a role na equipe. Editor ⊂ Revisor ⊂ Admin.
type Role string

const (
	RoleEditor   Role = "editor"
	RoleReviewer Role = "reviewer"
	RoleAdmin    Role = "admin"
)

// Roles lista as roles válidas, da menor para a maior.
func Roles() []Role { return []Role{RoleEditor, RoleReviewer, RoleAdmin} }

func ParseRole(s string) (Role, error) {
	for _, r := range Roles() {
		if Role(s) == r {
			return r, nil
		}
	}
	return "", ErrInvalidRole
}

// CanManageUsers: só o Admin cadastra, troca role, reseta senha e exclui.
func CanManageUsers(r Role) bool { return r == RoleAdmin }

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// User é uma pessoa da equipe. Não há estado pendente nem convite: o Admin
// cadastra a pessoa já com senha e role (a primeira Admin nasce pelo
// bootstrap).
type User struct {
	ID           string
	Name         string
	Email        string
	PasswordHash string
	Role         Role
	// MustChangePassword marca senha definida pelo Admin (temporária). Ainda
	// não é aplicada: a troca obrigatória é a issue #11.
	MustChangePassword bool
	// SessionsValidAfter: tokens emitidos antes deste instante são
	// recusados, então trocar ou resetar a senha encerra todas as sessões.
	SessionsValidAfter *time.Time
	CreatedAt          time.Time
}

// NewUser valida e monta um usuário. passwordHash já vem do hasher;
// mustChange indica senha definida por outra pessoa (o Admin).
func NewUser(id, name, email, passwordHash string, role Role, mustChange bool, now time.Time) (User, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return User{}, ErrInvalidName
	}
	email = strings.TrimSpace(email)
	if !emailPattern.MatchString(email) || len(email) > 254 {
		return User{}, ErrInvalidEmail
	}
	if _, err := ParseRole(string(role)); err != nil {
		return User{}, err
	}
	return User{
		ID: id, Name: name, Email: email, PasswordHash: passwordHash, Role: role,
		MustChangePassword: mustChange, CreatedAt: now,
	}, nil
}

// ValidatePassword confere só o tamanho: 8 caracteres no mínimo, sem regra
// de classes de caractere.
func ValidatePassword(plain string) error {
	if utf8.RuneCountInString(plain) < MinPasswordLen {
		return ErrPasswordTooShort
	}
	if len(plain) > maxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

// ChangePassword é a troca feita pela própria pessoa: a senha deixa de ser
// temporária e todas as sessões abertas acabam.
func (u *User) ChangePassword(hash string, now time.Time) {
	u.PasswordHash = hash
	u.MustChangePassword = false
	u.endSessions(now)
}

// ResetPassword é o reset feito pelo Admin: a nova senha é temporária e
// quem estava logado sai.
func (u *User) ResetPassword(hash string, now time.Time) {
	u.PasswordHash = hash
	u.MustChangePassword = true
	u.endSessions(now)
}

// endSessions trunca em milissegundos, a precisão do token de acesso.
func (u *User) endSessions(now time.Time) {
	t := now.UTC().Truncate(time.Millisecond)
	u.SessionsValidAfter = &t
}

// SessionValid diz se um token emitido em issuedAt ainda vale.
func (u User) SessionValid(issuedAt time.Time) bool {
	return u.SessionsValidAfter == nil || !issuedAt.Before(*u.SessionsValidAfter)
}
