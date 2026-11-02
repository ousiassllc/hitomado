-- name: UpsertReceivedContact :exec
INSERT INTO contacts (account_id, email, display_name, send_count, receive_count, last_used_at)
VALUES (sqlc.arg(account_id), sqlc.arg(email), sqlc.narg(display_name), 0, 1, sqlc.arg(last_used_at))
ON CONFLICT (account_id, email) DO UPDATE SET
    receive_count = contacts.receive_count + 1,
    display_name = COALESCE(excluded.display_name, contacts.display_name),
    last_used_at = MAX(contacts.last_used_at, excluded.last_used_at);

-- name: UpsertSentContact :exec
INSERT INTO contacts (account_id, email, display_name, send_count, receive_count, last_used_at)
VALUES (sqlc.arg(account_id), sqlc.arg(email), sqlc.narg(display_name), 1, 0, sqlc.arg(last_used_at))
ON CONFLICT (account_id, email) DO UPDATE SET
    send_count = contacts.send_count + 1,
    display_name = COALESCE(excluded.display_name, contacts.display_name),
    last_used_at = MAX(contacts.last_used_at, excluded.last_used_at);

-- name: SuggestContacts :many
SELECT c.email, c.display_name
FROM contacts c
WHERE c.account_id = sqlc.arg(account_id)
  AND ((c.email >= sqlc.arg(email_from) AND c.email < sqlc.arg(email_to))
       OR like(sqlc.arg(name_pattern), c.display_name, '\'))
  AND c.email NOT IN (SELECT a.email FROM accounts a WHERE a.id = c.account_id)
ORDER BY (c.send_count * 3 + c.receive_count) DESC, c.last_used_at DESC, c.email
LIMIT sqlc.arg(row_limit);
