-- name: GetSetting :one
SELECT value FROM app_settings WHERE key = sqlc.arg(key);

-- name: SetSetting :exec
INSERT INTO app_settings (key, value, updated_at)
VALUES (sqlc.arg(key), sqlc.arg(value), sqlc.arg(updated_at))
ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at;
