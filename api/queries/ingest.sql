-- Radar do módulo ingest (#45).

-- name: UpsertRadarItem :exec
-- A pauta mais recente e a data de apresentação ficam; dispensa não muda.
INSERT INTO ingest.radar_items (chamber, external_id, kind, number, year, summary, source_url,
                                agenda_at, presented_at, first_seen_at, last_seen_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (chamber, external_id) DO UPDATE
SET kind = EXCLUDED.kind,
    number = EXCLUDED.number,
    year = EXCLUDED.year,
    summary = CASE WHEN EXCLUDED.summary <> '' THEN EXCLUDED.summary ELSE ingest.radar_items.summary END,
    source_url = CASE WHEN EXCLUDED.source_url <> '' THEN EXCLUDED.source_url ELSE ingest.radar_items.source_url END,
    agenda_at = GREATEST(ingest.radar_items.agenda_at, EXCLUDED.agenda_at),
    presented_at = COALESCE(ingest.radar_items.presented_at, EXCLUDED.presented_at),
    last_seen_at = EXCLUDED.last_seen_at;

-- name: ListVisibleRadarItems :many
-- Pauta a partir de agenda_since, ou apresentada desde presented_since;
-- dispensada só volta com pauta nova.
SELECT * FROM ingest.radar_items
WHERE (agenda_at >= sqlc.arg(agenda_since) OR presented_at >= sqlc.arg(presented_since))
  AND (dismissed_at IS NULL OR agenda_at > COALESCE(dismissed_agenda_at, '-infinity'::timestamptz))
ORDER BY agenda_at NULLS LAST, presented_at DESC, chamber, external_id
LIMIT 500;

-- name: DismissRadarItem :one
UPDATE ingest.radar_items
SET dismissed_at = sqlc.arg(now), dismissed_by = sqlc.arg(by), dismissed_agenda_at = agenda_at
WHERE chamber = $1 AND external_id = $2
RETURNING *;

-- name: PurgeRadarItems :execrows
DELETE FROM ingest.radar_items WHERE last_seen_at < sqlc.arg(before);

-- name: RadarLastSeen :one
SELECT last_seen_at FROM ingest.radar_items ORDER BY last_seen_at DESC LIMIT 1;
