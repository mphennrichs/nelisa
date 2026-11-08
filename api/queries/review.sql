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
