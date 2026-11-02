-- name: GetSyncState :one
SELECT account_id, history_id, inbox_page_token, inbox_exhausted, initial_sync_done, last_synced_at, last_error
FROM account_sync_state
WHERE account_id = sqlc.arg(account_id);

-- name: SaveSyncState :execrows
UPDATE account_sync_state
SET history_id = sqlc.narg(history_id), inbox_page_token = sqlc.narg(inbox_page_token),
    inbox_exhausted = sqlc.arg(inbox_exhausted), initial_sync_done = sqlc.arg(initial_sync_done),
    last_synced_at = sqlc.narg(last_synced_at), last_error = sqlc.narg(last_error)
WHERE account_id = sqlc.arg(account_id);
