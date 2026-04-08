-- name: CreateClient :one
INSERT INTO clients (id, name, api_key, created_at, updated_at)
VALUES ($1, $2, $3, NOW(), NOW())
RETURNING *;

-- name: GetClientById :one
SELECT * FROM clients WHERE id = $1;    

-- name: GetClientByApiKey :one
SELECT * FROM clients WHERE api_key = $1;

-- name: ListClients :many
SELECT * FROM clients ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: UpdateClient :one
UPDATE clients SET name = $1, updated_at = NOW() WHERE id = $2 RETURNING *;