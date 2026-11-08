-- Consultas do módulo queue (Posse).

-- name: LockClaim :exec
-- Serializa quem assume, grava ou solta a Posse de uma Proposição até o
-- fim da transação, exista a linha ou não (FOR UPDATE não trava linha que
-- ainda não existe).
SELECT pg_advisory_xact_lock(hashtextextended('queue.claim:' || sqlc.arg(proposition_id)::text, 0));

-- name: GetClaim :one
SELECT * FROM queue.claims WHERE proposition_id = $1;

-- name: ListClaims :many
SELECT * FROM queue.claims WHERE proposition_id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY proposition_id;

-- name: UpsertClaim :exec
INSERT INTO queue.claims (proposition_id, user_id, claimed_at, last_activity_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (proposition_id) DO UPDATE
SET user_id = EXCLUDED.user_id, claimed_at = EXCLUDED.claimed_at, last_activity_at = EXCLUDED.last_activity_at;

-- name: DeleteClaim :exec
DELETE FROM queue.claims WHERE proposition_id = $1;
