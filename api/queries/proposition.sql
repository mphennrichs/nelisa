-- Consultas do módulo proposition (Proposição e Vínculo de fonte).

-- name: CreateProposition :exec
INSERT INTO proposition.propositions (
    id, slug, kind, number, year, sphere, origin_chamber,
    official_summary, presented_at, official_text_url, authors, status, status_at,
    attached_to, attached_to_name, superseded, review_state, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13,
    $14, $15, $16, $17, $18, $19
);

-- name: GetPropositionByID :one
SELECT * FROM proposition.propositions WHERE id = $1;

-- name: GetPropositionByIDForUpdate :one
-- Serializa mudanças concorrentes na mesma Proposição (duas
-- Sincronizações, ou Sincronização e envio para revisão).
SELECT * FROM proposition.propositions WHERE id = $1 FOR UPDATE;

-- name: GetPropositionBySlug :one
SELECT * FROM proposition.propositions WHERE slug = $1;

-- name: SlugExists :one
SELECT EXISTS (SELECT 1 FROM proposition.propositions WHERE slug = $1);

-- name: FindPropositionsByName :many
SELECT * FROM proposition.propositions
WHERE kind = $1 AND number = $2 AND year = $3
ORDER BY created_at, id;

-- name: ListPropositions :many
SELECT * FROM proposition.propositions ORDER BY updated_at DESC, id;

-- name: ListPropositionsInStates :many
-- A parte pública passa os estados visíveis (domain.PublicStates); a
-- regra mora no domínio, não aqui.
SELECT * FROM proposition.propositions
WHERE review_state = ANY(sqlc.arg(states)::text[])
ORDER BY updated_at DESC, id;

-- name: ListAttachedTo :many
SELECT * FROM proposition.propositions WHERE attached_to = $1 ORDER BY year, number, id;

-- name: ListUnresolvedAttachments :many
-- Apensadas cuja principal ainda não estava cadastrada: quando ela chega,
-- a Apensação é resolvida.
SELECT * FROM proposition.propositions
WHERE attached_to IS NULL AND attached_to_name = $1;

-- name: UpdateProposition :exec
UPDATE proposition.propositions
SET official_summary = $2,
    presented_at = $3,
    official_text_url = $4,
    authors = $5,
    status = $6,
    status_at = $7,
    attached_to = $8,
    attached_to_name = $9,
    superseded = $10,
    review_state = $11,
    updated_at = $12
WHERE id = $1;

-- name: CreateLink :exec
INSERT INTO proposition.source_links (
    id, proposition_id, chamber, external_id, state, source_name, source_url,
    confirmed_by, confirmed_at, last_synced_at, last_sync_error, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: GetLink :one
SELECT * FROM proposition.source_links WHERE id = $1;

-- name: ListLinksByProposition :many
SELECT * FROM proposition.source_links WHERE proposition_id = $1 ORDER BY chamber, created_at, id;

-- name: ListLinksByPropositions :many
SELECT * FROM proposition.source_links
WHERE proposition_id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY proposition_id, chamber, created_at, id;

-- name: ListLinksByExternal :many
-- Vínculos (sugeridos ou confirmados) de registros de uma Casa: a busca
-- marca o que já está no Nelisa.
SELECT * FROM proposition.source_links
WHERE chamber = $1 AND external_id = ANY(sqlc.arg(external_ids)::text[])
ORDER BY state, created_at, id;

-- name: ListConfirmedLinkIDs :many
SELECT id FROM proposition.source_links WHERE state = 'confirmed' ORDER BY id;

-- name: ConfirmLink :execrows
-- Só confirma o que ainda é sugestão.
UPDATE proposition.source_links
SET state = 'confirmed', confirmed_by = $2, confirmed_at = $3
WHERE id = $1 AND state = 'suggested';

-- name: DeleteSuggestedLink :execrows
DELETE FROM proposition.source_links WHERE id = $1 AND state = 'suggested';

-- name: UpdateLinkSync :exec
UPDATE proposition.source_links
SET last_synced_at = $2, last_sync_error = $3, source_name = $4, source_url = $5
WHERE id = $1;

-- Versões e conteúdo editorial.

-- name: CreateVersion :exec
INSERT INTO proposition.versions (
    id, proposition_id, seq, label, official_text_url, state, created_by, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetVersion :one
SELECT * FROM proposition.versions WHERE id = $1;

-- name: ListVersionsByProposition :many
SELECT * FROM proposition.versions WHERE proposition_id = $1 ORDER BY seq;

-- name: ListVersionsByPropositions :many
SELECT * FROM proposition.versions
WHERE proposition_id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY proposition_id, seq;

-- name: UpdateVersionState :execrows
-- Só muda se ainda estiver no estado esperado.
UPDATE proposition.versions SET state = sqlc.arg(state)
WHERE id = sqlc.arg(id) AND state = sqlc.arg(from_state);

-- name: CreateContent :exec
INSERT INTO proposition.version_contents (
    version_id, summary, short_summary, topic_ids, needs_confirmation, authors, revision, updated_by, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetContent :one
SELECT * FROM proposition.version_contents WHERE version_id = $1;

-- name: ListContentsByVersions :many
SELECT * FROM proposition.version_contents
WHERE version_id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY version_id;

-- name: UpdateContent :execrows
-- Só grava se ninguém gravou depois da leitura (trava otimista).
UPDATE proposition.version_contents
SET summary = sqlc.arg(summary),
    short_summary = sqlc.arg(short_summary),
    topic_ids = sqlc.arg(topic_ids),
    needs_confirmation = sqlc.arg(needs_confirmation),
    authors = sqlc.arg(authors),
    revision = sqlc.arg(revision),
    updated_by = sqlc.arg(updated_by),
    updated_at = sqlc.arg(updated_at)
WHERE version_id = sqlc.arg(version_id) AND revision = sqlc.arg(prev_revision);

-- Temas.

-- name: CreateTopic :exec
INSERT INTO proposition.topics (id, name, active, created_at) VALUES ($1, $2, $3, $4);

-- name: GetTopic :one
SELECT * FROM proposition.topics WHERE id = $1;

-- name: ListTopics :many
SELECT * FROM proposition.topics ORDER BY lower(name), id;

-- name: UpdateTopic :exec
UPDATE proposition.topics SET name = $2, active = $3 WHERE id = $1;
