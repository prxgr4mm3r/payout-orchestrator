-- name: CreateOutboxEvent :one
INSERT INTO outbox_events (event_type, entity_id, payload)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ClaimNextPendingOutboxEvent :one
WITH next_event AS (
    SELECT id
    FROM outbox_events
    WHERE status = 'pending'
       OR (status = 'processing' AND outbox_events.claimed_at < sqlc.arg(reclaim_before))
    ORDER BY created_at ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE outbox_events AS events
SET status = 'processing',
    claimed_at = NOW()
FROM next_event
WHERE events.id = next_event.id
RETURNING events.*;

-- name: GetPendingOutboxEvents :many
SELECT * FROM outbox_events WHERE status = 'pending' ORDER BY created_at ASC;

-- name: MarkOutboxEventAsProcessed :one
UPDATE outbox_events
SET status = 'processed',
    processed_at = NOW(),
    claimed_at = NULL
WHERE id = $1
RETURNING *;

-- name: ReleaseOutboxEventClaim :one
UPDATE outbox_events
SET status = 'pending',
    claimed_at = NULL
WHERE id = $1
RETURNING *;
