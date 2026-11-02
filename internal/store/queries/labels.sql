-- name: ListLabels :many
SELECT id, name, type, color_bg, color_fg
FROM labels
WHERE account_id = sqlc.arg(account_id)
ORDER BY type, name, id;

-- name: DeleteLabels :exec
DELETE FROM labels WHERE account_id = sqlc.arg(account_id);

-- name: InsertLabel :exec
INSERT INTO labels (account_id, id, name, type, color_bg, color_fg)
VALUES (sqlc.arg(account_id), sqlc.arg(id), sqlc.arg(name), sqlc.arg(type), sqlc.narg(color_bg), sqlc.narg(color_fg))
ON CONFLICT (account_id, id) DO UPDATE
SET name = excluded.name, type = excluded.type, color_bg = excluded.color_bg, color_fg = excluded.color_fg;
