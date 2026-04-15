-- name: CreateFundingSource :one
INSERT INTO funding_sources (client_id, name, type, payment_account_id)
VALUES ($1, $2, $3, $4)
RETURNING id, client_id, name, type, payment_account_id, status, created_at, updated_at;

-- name: GetFundingSourceByClientID :one
SELECT id, client_id, name, type, payment_account_id, status, created_at, updated_at
FROM funding_sources
WHERE client_id = $1 AND id = $2;

-- name: ListFundingSourcesByClientID :many
SELECT id, client_id, name, type, payment_account_id, status, created_at, updated_at
FROM funding_sources
WHERE client_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;
