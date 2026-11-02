-- name: ListOAuthClients :many
SELECT c.id, c.name, c.client_id, c.created_at, c.updated_at,
       (SELECT COUNT(*) FROM accounts a WHERE a.oauth_client_id = c.id) AS account_count
FROM oauth_clients c
ORDER BY c.created_at, c.id;

-- name: GetOAuthClient :one
SELECT id, name, client_id, created_at, updated_at
FROM oauth_clients
WHERE id = sqlc.arg(id);

-- name: CreateOAuthClient :exec
INSERT INTO oauth_clients (id, name, client_id, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(client_id), sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: UpdateOAuthClient :execrows
UPDATE oauth_clients
SET name = sqlc.arg(name), client_id = sqlc.arg(client_id), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteOAuthClient :execrows
DELETE FROM oauth_clients WHERE id = sqlc.arg(id);
