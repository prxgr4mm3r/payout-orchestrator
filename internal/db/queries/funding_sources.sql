-- name: CreateFundingSource :one
INSERT INTO funding_sources (id, client_id, name, type, payment_account_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
RETURNING *;

-- name: GetFundingSourceById :one
SELECT * FROM funding_sources WHERE id = $1;

-- name: ListFundingSourcesByClientId :many
SELECT * FROM funding_sources WHERE client_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: UpdateFundingSource :one
UPDATE funding_sources SET name = $1, type = $2, payment_account_id = $3, updated_at = NOW() WHERE id = $4 RETURNING *; 

