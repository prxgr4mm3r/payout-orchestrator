-- name: CreateOutboxEvent :one
INSERT INTO outbox_events (id, event_type, payload, created_at)
VALUES ($1, $2, $3, NOW())
RETURNING *;

-- name: GetPendingOutboxEvents :many
SELECT * FROM outbox_events WHERE status = 'pending' ORDER BY created_at ASC;

-- name: MarkOutboxEventAsProcessed :one
UPDATE outbox_events SET status = 'processed', processed_at = NOW() WHERE id = $1 RETURNING *;