-- Contribuições e Fonte oficial (módulo contribution).

-- name: InsertContribution :exec
INSERT INTO contribution.contributions (id, kind, state, tracking_hash, proposition_id, sphere, payload, submitted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetContributionByTrackingHash :one
SELECT * FROM contribution.contributions WHERE tracking_hash = $1;

-- name: GetContribution :one
SELECT * FROM contribution.contributions WHERE id = $1;

-- name: GetContributionForUpdate :one
SELECT * FROM contribution.contributions WHERE id = $1 FOR UPDATE;

-- name: ListPendingContributions :many
SELECT * FROM contribution.contributions WHERE state = 'pending' ORDER BY submitted_at, id;

-- name: UpdateContribution :execrows
-- Moderação: só os campos que a equipe muda.
UPDATE contribution.contributions
SET state = $2, sphere = $3, decided_by = $4, decided_at = $5, public_reason = $6,
    internal_note = $7, created_proposition_id = $8
WHERE id = $1;

-- name: CountPendingContributions :one
SELECT count(*) FROM contribution.contributions WHERE state = 'pending';

-- name: ListOfficialSources :many
SELECT * FROM contribution.official_sources ORDER BY domain;

-- name: ListOfficialDomains :many
SELECT domain FROM contribution.official_sources;

-- name: InsertOfficialSource :exec
INSERT INTO contribution.official_sources (id, domain, name, sphere, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: DeleteOfficialSource :one
DELETE FROM contribution.official_sources WHERE id = $1 RETURNING *;
