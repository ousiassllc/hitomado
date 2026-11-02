-- name: ListAccounts :many
SELECT id, oauth_client_id, email, display_name, color, signature_html, notify_enabled,
       status, status_message, sort_order, created_at, updated_at
FROM accounts
ORDER BY sort_order, created_at, id;

-- name: GetAccount :one
SELECT id, oauth_client_id, email, display_name, color, signature_html, notify_enabled,
       status, status_message, sort_order, created_at, updated_at
FROM accounts
WHERE id = sqlc.arg(id);

-- name: CreateAccount :exec
INSERT INTO accounts (id, oauth_client_id, email, display_name, color, signature_html, notify_enabled,
                      status, status_message, sort_order, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(oauth_client_id), sqlc.arg(email), sqlc.arg(display_name), sqlc.arg(color),
        sqlc.arg(signature_html), sqlc.arg(notify_enabled), sqlc.arg(status), sqlc.narg(status_message),
        sqlc.arg(sort_order), sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: CreateSyncState :exec
INSERT INTO account_sync_state (account_id) VALUES (sqlc.arg(account_id));

-- name: UpdateAccount :execrows
UPDATE accounts
SET display_name = sqlc.arg(display_name), color = sqlc.arg(color),
    notify_enabled = sqlc.arg(notify_enabled), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: SetAccountStatus :execrows
UPDATE accounts
SET status = sqlc.arg(status), status_message = sqlc.narg(status_message), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: SetAccountSignature :execrows
UPDATE accounts
SET signature_html = sqlc.arg(signature_html), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: SetAccountSortOrder :execrows
UPDATE accounts SET sort_order = sqlc.arg(sort_order) WHERE id = sqlc.arg(id);

-- name: ListAccountIDs :many
SELECT id FROM accounts;

-- name: MaxAccountSortOrder :one
SELECT CAST(COALESCE(MAX(sort_order), -1) AS INTEGER) AS max_sort_order FROM accounts;

-- name: DeleteAccount :execrows
DELETE FROM accounts WHERE id = sqlc.arg(id);
