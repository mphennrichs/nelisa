-- Contribuições e Fonte oficial (módulo contribution).

-- name: InsertContribution :exec
INSERT INTO contribution.contributions (id, kind, state, tracking_hash, proposition_id, sphere, payload, submitted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetContributionByTrackingHash :one
SELECT * FROM contribution.contributions WHERE tracking_hash = $1;

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
