-- name: CreateOutboxEvent :one
INSERT INTO outbox_events (event_type, entity_id, payload)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetPendingOutboxEvents :many
SELECT * FROM outbox_events WHERE status = 'pending' ORDER BY created_at ASC;

-- name: MarkOutboxEventAsProcessed :one
UPDATE outbox_events SET status = 'processed', processed_at = NOW() WHERE id = $1 RETURNING *;
