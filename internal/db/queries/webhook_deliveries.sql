-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (
    payout_id,
    client_id,
    target_url,
    payload,
    status,
    attempt_count
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetWebhookDelivery :one
SELECT *
FROM webhook_deliveries
WHERE id = $1;

-- name: ListWebhookDeliveriesByPayoutID :many
SELECT *
FROM webhook_deliveries
WHERE payout_id = $1
ORDER BY created_at DESC, id DESC;
