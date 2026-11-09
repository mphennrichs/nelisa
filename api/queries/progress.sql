-- Consultas do módulo progress (Tramitação).

-- name: UpsertEntry :one
-- Idempotente: o mesmo evento (Proposição, Casa, chave) é atualizado, não
-- duplicado. inserted diz se a linha é nova (xmax = 0 só na inserção).
INSERT INTO progress.entries (proposition_id, chamber, external_key, at, body, description, situation, dispatch, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (proposition_id, chamber, external_key) DO UPDATE
SET at = EXCLUDED.at,
    body = EXCLUDED.body,
    description = EXCLUDED.description,
    situation = EXCLUDED.situation,
    dispatch = EXCLUDED.dispatch,
    updated_at = EXCLUDED.updated_at
-- Evento igual ao guardado: nada a fazer (e nenhuma linha volta).
WHERE (progress.entries.at, progress.entries.body, progress.entries.description, progress.entries.situation, progress.entries.dispatch)
    IS DISTINCT FROM (EXCLUDED.at, EXCLUDED.body, EXCLUDED.description, EXCLUDED.situation, EXCLUDED.dispatch)
RETURNING (xmax = 0)::boolean AS inserted;

-- name: ListEntriesByProposition :many
SELECT proposition_id, chamber, external_key, at, body, description, situation, dispatch, manual, source_url
FROM progress.entries
WHERE proposition_id = $1
ORDER BY at, chamber, external_key;

-- name: InsertManualEntry :execrows
-- Evento manual (Atualização de tramitação aceita). Se a chave já existe,
-- nada muda: 0 linhas.
INSERT INTO progress.entries (proposition_id, chamber, external_key, at, body, description, manual, source_url, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, true, $7, sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (proposition_id, chamber, external_key) DO NOTHING;

-- name: DeleteManualEntry :execrows
DELETE FROM progress.entries WHERE external_key = $1 AND manual;
