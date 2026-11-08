-- Consultas do módulo review (aprovações e segunda revisão).

-- name: CreateApproval :exec
INSERT INTO review.approvals (
    version_id, proposition_id, approved_by, approved_at, four_eyes, second_review_by, second_review_at
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetApproval :one
SELECT * FROM review.approvals WHERE version_id = $1;

-- name: GetApprovalForUpdate :one
SELECT * FROM review.approvals WHERE version_id = $1 FOR UPDATE;

-- name: ListApprovalsByVersions :many
SELECT * FROM review.approvals
WHERE version_id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY version_id;

-- name: ListPendingSecondReviews :many
SELECT * FROM review.approvals
WHERE NOT four_eyes AND second_review_by IS NULL
ORDER BY approved_at, version_id;

-- name: SetSecondReview :execrows
-- Só uma segunda revisão por aprovação, e só onde faltou o quatro olhos.
UPDATE review.approvals
SET second_review_by = $2, second_review_at = $3
WHERE version_id = $1 AND NOT four_eyes AND second_review_by IS NULL;

-- Correções menores.

-- name: CreateMinorFix :exec
INSERT INTO review.minor_fixes (
    id, proposition_id, version_id, fields, before, after, created_by, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetMinorFixForUpdate :one
SELECT * FROM review.minor_fixes WHERE id = $1 FOR UPDATE;

-- name: SetMinorFixChecked :execrows
-- Só uma conferência por correção.
UPDATE review.minor_fixes
SET checked_by = $2, checked_at = $3
WHERE id = $1 AND checked_by IS NULL;

-- name: ListUncheckedMinorFixesByProposition :many
SELECT * FROM review.minor_fixes
WHERE proposition_id = $1 AND checked_by IS NULL
ORDER BY created_at, id;

-- name: ListUncheckedMinorFixes :many
SELECT * FROM review.minor_fixes
WHERE checked_by IS NULL
ORDER BY created_at, id;

-- Erratas.

-- name: CreateErratum :exec
INSERT INTO review.errata (
    id, proposition_id, version_id, proposed, said, correct, base_revision, state, created_by, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8, $9);

-- name: GetErratumForUpdate :one
SELECT * FROM review.errata WHERE id = $1 FOR UPDATE;

-- name: DecideErratum :execrows
-- Só uma decisão por Errata: a pendente.
UPDATE review.errata
SET state = $2, decided_by = $3, decided_at = $4, reason = $5
WHERE id = $1 AND state = 'pending';

-- name: CountPendingErrataByVersion :one
SELECT count(*) FROM review.errata WHERE version_id = $1 AND state = 'pending';

-- name: ListErrataByProposition :many
SELECT * FROM review.errata
WHERE proposition_id = $1
ORDER BY created_at DESC, id;

-- name: ListPendingErrata :many
SELECT * FROM review.errata
WHERE state = 'pending'
ORDER BY created_at, id;

-- name: ListApprovedErrataByVersions :many
SELECT * FROM review.errata
WHERE state = 'approved' AND version_id = ANY(sqlc.arg(ids)::uuid[])
ORDER BY decided_at DESC, id;
