-- name: CreateIdempotencyKey :one
INSERT INTO idempotency_keys (key, client_id, request_hash, payout_id)
VALUES ($1, $2, $3, $4)
RETURNING key, client_id, request_hash, payout_id, created_at;

-- name: GetIdempotencyKey :one
SELECT key, client_id, request_hash, payout_id, created_at
FROM idempotency_keys
WHERE client_id = $1 AND key = $2;
