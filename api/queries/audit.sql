-- Consultas do módulo audit (Auditoria). Só inserção e leitura: eventos
-- nunca são alterados nem apagados.

-- name: InsertEvent :exec
INSERT INTO audit.events (actor_id, action, target_type, target_id, at, metadata)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListEventsByTarget :many
SELECT id, actor_id, action, target_type, target_id, at, metadata
FROM audit.events
WHERE target_type = $1 AND target_id = $2
ORDER BY at, id;
