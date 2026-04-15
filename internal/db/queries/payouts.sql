-- name: CreatePayout :one
INSERT INTO payouts (client_id, funding_source_id, amount, currency)
VALUES ($1, $2, $3, $4)
RETURNING id, client_id, funding_source_id, amount, currency, status, created_at, updated_at;

-- name: GetPayoutByClientID :one
SELECT id, client_id, funding_source_id, amount, currency, status, created_at, updated_at
FROM payouts
WHERE client_id = $1 AND id = $2;

-- name: ListPayoutsByClientID :many
SELECT id, client_id, funding_source_id, amount, currency, status, created_at, updated_at
FROM payouts
WHERE client_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: UpdatePayoutStatus :one
UPDATE payouts
SET status = $1, updated_at = NOW()
WHERE id = $2
RETURNING id, client_id, funding_source_id, amount, currency, status, created_at, updated_at;
