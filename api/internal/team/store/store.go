// Package store é o adaptador Postgres das portas de team/app. Toda
// consulta usa a transação do contexto quando há uma (db.Conn).
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mphennrichs/nelisa/api/internal/platform/db"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
	"github.com/mphennrichs/nelisa/api/internal/team/store/dbgen"
)

const (
	pgUniqueViolation = "23505"
	// pgInvalidText: um id que não é UUID. Para quem pergunta, é "não existe".
	pgInvalidText = "22P02"
)

// Users implementa app.UserRepo.
type Users struct{ pool *pgxpool.Pool }

func NewUsers(pool *pgxpool.Pool) *Users { return &Users{pool: pool} }

func (s *Users) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *Users) LockUsers(ctx context.Context) error {
	if err := s.q(ctx).LockUsers(ctx); err != nil {
		return fmt.Errorf("travando usuários: %w", err)
	}
	return nil
}

func (s *Users) Count(ctx context.Context) (int, error) {
	n, err := s.q(ctx).CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("contando usuários: %w", err)
	}
	return int(n), nil
}

func (s *Users) CountAdmins(ctx context.Context) (int, error) {
	n, err := s.q(ctx).CountAdmins(ctx)
	if err != nil {
		return 0, fmt.Errorf("contando admins: %w", err)
	}
	return int(n), nil
}

func (s *Users) Create(ctx context.Context, u domain.User) error {
	err := s.q(ctx).CreateUser(ctx, dbgen.CreateUserParams{
		ID: u.ID, Name: u.Name, Email: u.Email, PasswordHash: u.PasswordHash, Role: string(u.Role),
		MustChangePassword: u.MustChangePassword, SessionsValidAfter: u.SessionsValidAfter, CreatedAt: u.CreatedAt,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == "users_email_key" {
		return domain.ErrEmailTaken
	}
	if err != nil {
		return fmt.Errorf("criando usuário: %w", err)
	}
	return nil
}

func (s *Users) FindByID(ctx context.Context, id string) (domain.User, error) {
	row, err := s.q(ctx).GetUserByID(ctx, id)
	return userOrNotFound(row, err)
}

func (s *Users) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := s.q(ctx).GetUserByEmail(ctx, email)
	return userOrNotFound(row, err)
}

func (s *Users) List(ctx context.Context) ([]domain.User, error) {
	rows, err := s.q(ctx).ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("listando usuários: %w", err)
	}
	out := make([]domain.User, 0, len(rows))
	for _, r := range rows {
		out = append(out, toUser(r))
	}
	return out, nil
}

func (s *Users) UpdatePassword(ctx context.Context, u domain.User, prevHash string) error {
	n, err := s.q(ctx).UpdatePassword(ctx, dbgen.UpdatePasswordParams{
		ID: u.ID, PasswordHash: u.PasswordHash, MustChangePassword: u.MustChangePassword,
		SessionsValidAfter: u.SessionsValidAfter, PrevHash: prevHash,
	})
	if err != nil {
		return notFoundOr(err, "gravando senha")
	}
	if n == 0 {
		if _, err := s.FindByID(ctx, u.ID); err != nil {
			return err
		}
		return domain.ErrCredentialsChanged
	}
	return nil
}

func (s *Users) UpdateRole(ctx context.Context, id string, role domain.Role) (domain.User, error) {
	row, err := s.q(ctx).UpdateRole(ctx, dbgen.UpdateRoleParams{ID: id, Role: string(role)})
	return userOrNotFound(row, err)
}

func (s *Users) UpdateDeactivation(ctx context.Context, u domain.User) error {
	n, err := s.q(ctx).UpdateDeactivation(ctx, dbgen.UpdateDeactivationParams{
		ID: u.ID, DeactivatedAt: u.DeactivatedAt, SessionsValidAfter: u.SessionsValidAfter,
	})
	if err != nil {
		return notFoundOr(err, "gravando desativação")
	}
	if n == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func userOrNotFound(row dbgen.TeamUser, err error) (domain.User, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, notFoundOr(err, "buscando usuário")
	}
	return toUser(row), nil
}

func notFoundOr(err error, what string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgInvalidText {
		return domain.ErrUserNotFound
	}
	return fmt.Errorf("%s: %w", what, err)
}

func toUser(r dbgen.TeamUser) domain.User {
	u := domain.User{
		ID: r.ID, Name: r.Name, Email: r.Email, PasswordHash: r.PasswordHash, Role: domain.Role(r.Role),
		MustChangePassword: r.MustChangePassword, CreatedAt: r.CreatedAt.UTC(),
	}
	if r.SessionsValidAfter != nil {
		t := r.SessionsValidAfter.UTC()
		u.SessionsValidAfter = &t
	}
	if r.DeactivatedAt != nil {
		t := r.DeactivatedAt.UTC()
		u.DeactivatedAt = &t
	}
	return u
}

// RefreshTokens implementa app.RefreshTokenRepo.
type RefreshTokens struct{ pool *pgxpool.Pool }

func NewRefreshTokens(pool *pgxpool.Pool) *RefreshTokens { return &RefreshTokens{pool: pool} }

func (s *RefreshTokens) q(ctx context.Context) *dbgen.Queries { return dbgen.New(db.Conn(ctx, s.pool)) }

func (s *RefreshTokens) Create(ctx context.Context, t domain.RefreshToken) error {
	err := s.q(ctx).CreateRefreshToken(ctx, dbgen.CreateRefreshTokenParams{
		ID: t.ID, UserID: t.UserID, TokenHash: t.TokenHash, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt,
	})
	if err != nil {
		return fmt.Errorf("gravando refresh token: %w", err)
	}
	return nil
}

func (s *RefreshTokens) FindByHash(ctx context.Context, hash string) (domain.RefreshToken, error) {
	r, err := s.q(ctx).GetRefreshTokenByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RefreshToken{}, domain.ErrRefreshTokenInvalid
	}
	if err != nil {
		return domain.RefreshToken{}, fmt.Errorf("buscando refresh token: %w", err)
	}
	t := domain.RefreshToken{
		ID: r.ID, UserID: r.UserID, TokenHash: r.TokenHash, CreatedAt: r.CreatedAt.UTC(), ExpiresAt: r.ExpiresAt.UTC(),
	}
	if r.RevokedAt != nil {
		at := r.RevokedAt.UTC()
		t.RevokedAt = &at
	}
	return t, nil
}

func (s *RefreshTokens) Revoke(ctx context.Context, id string, at time.Time) error {
	n, err := s.q(ctx).RevokeRefreshToken(ctx, dbgen.RevokeRefreshTokenParams{ID: id, RevokedAt: &at})
	if err != nil {
		return fmt.Errorf("revogando refresh token: %w", err)
	}
	if n == 1 {
		return nil
	}
	exists, err := s.q(ctx).RefreshTokenExists(ctx, id)
	if err != nil {
		return fmt.Errorf("conferindo refresh token: %w", err)
	}
	if !exists {
		return domain.ErrRefreshTokenInvalid
	}
	return domain.ErrRefreshTokenReused
}

func (s *RefreshTokens) Delete(ctx context.Context, id string) error {
	if err := s.q(ctx).DeleteRefreshToken(ctx, id); err != nil {
		return fmt.Errorf("apagando refresh token: %w", err)
	}
	return nil
}

func (s *RefreshTokens) DeleteAllForUser(ctx context.Context, userID string) error {
	if err := s.q(ctx).DeleteRefreshTokensForUser(ctx, userID); err != nil {
		return fmt.Errorf("apagando sessões: %w", err)
	}
	return nil
}
