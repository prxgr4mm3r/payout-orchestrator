-- name: CreatePayout :one
INSERT INTO payouts (id, client_id, funding_source_id, amount, currency, status, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
RETURNING *;

-- name: GetPayoutById :one
SELECT * FROM payouts WHERE id = $1;

-- name: ListPayoutsByClientId :many
SELECT * FROM payouts WHERE client_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: UpdatePayoutStatus :one
UPDATE payouts SET status = $1, updated_at = NOW() WHERE id = $2 RETURNING *;