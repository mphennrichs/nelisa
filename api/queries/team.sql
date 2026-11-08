-- Consultas do módulo team (cadastro de equipe e sessões).

-- name: LockUsers :exec
-- Serializa o bootstrap e as mudanças de quem é Admin. Só vale dentro de
-- transação (pg_advisory_xact_lock é solto no fim dela).
SELECT pg_advisory_xact_lock(7364526100001);

-- name: CountUsers :one
SELECT count(*) FROM team.users;

-- name: CountAdmins :one
-- Só Admins ativos: um Admin desativado não administra nada.
SELECT count(*) FROM team.users WHERE role = 'admin' AND deactivated_at IS NULL;

-- name: CreateUser :exec
INSERT INTO team.users (id, name, email, password_hash, role, must_change_password, sessions_valid_after, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetUserByID :one
SELECT id, name, email, password_hash, role, must_change_password, sessions_valid_after, deactivated_at, created_at
FROM team.users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, name, email, password_hash, role, must_change_password, sessions_valid_after, deactivated_at, created_at
FROM team.users WHERE lower(email) = lower(sqlc.arg(email));

-- name: ListUsers :many
SELECT id, name, email, password_hash, role, must_change_password, sessions_valid_after, deactivated_at, created_at
FROM team.users ORDER BY created_at, id;

-- name: UpdatePassword :execrows
-- Só grava se o hash ainda for o lido antes: duas trocas simultâneas não
-- desfazem uma à outra.
UPDATE team.users
SET password_hash = sqlc.arg(password_hash),
    must_change_password = sqlc.arg(must_change_password),
    sessions_valid_after = sqlc.arg(sessions_valid_after)
WHERE id = sqlc.arg(id) AND password_hash = sqlc.arg(prev_hash);

-- name: UpdateRole :one
UPDATE team.users SET role = $2 WHERE id = $1
RETURNING id, name, email, password_hash, role, must_change_password, sessions_valid_after, deactivated_at, created_at;

-- name: UpdateDeactivation :execrows
-- Desativar ou reativar: a conta nunca é apagada.
UPDATE team.users
SET deactivated_at = sqlc.arg(deactivated_at),
    sessions_valid_after = sqlc.arg(sessions_valid_after)
WHERE id = sqlc.arg(id);

-- name: CreateRefreshToken :exec
INSERT INTO team.refresh_tokens (id, user_id, token_hash, created_at, expires_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetRefreshTokenByHash :one
SELECT id, user_id, token_hash, created_at, expires_at, revoked_at
FROM team.refresh_tokens WHERE token_hash = $1;

-- name: RevokeRefreshToken :execrows
UPDATE team.refresh_tokens SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: RefreshTokenExists :one
SELECT EXISTS (SELECT 1 FROM team.refresh_tokens WHERE id = $1);

-- name: DeleteRefreshToken :exec
DELETE FROM team.refresh_tokens WHERE id = $1;

-- name: DeleteRefreshTokensForUser :exec
DELETE FROM team.refresh_tokens WHERE user_id = $1;
