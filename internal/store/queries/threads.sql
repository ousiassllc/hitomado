-- name: GetThreadMeta :one
SELECT in_inbox, left_inbox_at
FROM threads
WHERE account_id = sqlc.arg(account_id) AND id = sqlc.arg(id);

-- name: ListThreadMessageIDs :many
SELECT id FROM messages
WHERE account_id = sqlc.arg(account_id) AND thread_id = sqlc.arg(thread_id);

-- name: UpsertThread :exec
INSERT INTO threads (account_id, id, subject, snippet, last_message_at, last_from_name, last_from_email,
                     message_count, is_unread, has_attachment, in_inbox, left_inbox_at, history_id)
VALUES (sqlc.arg(account_id), sqlc.arg(id), sqlc.arg(subject), sqlc.arg(snippet), sqlc.arg(last_message_at),
        sqlc.narg(last_from_name), sqlc.arg(last_from_email), sqlc.arg(message_count), sqlc.arg(is_unread),
        sqlc.arg(has_attachment), sqlc.arg(in_inbox), sqlc.narg(left_inbox_at), sqlc.arg(history_id))
ON CONFLICT (account_id, id) DO UPDATE SET
    subject = excluded.subject,
    snippet = excluded.snippet,
    last_message_at = excluded.last_message_at,
    last_from_name = excluded.last_from_name,
    last_from_email = excluded.last_from_email,
    message_count = excluded.message_count,
    is_unread = excluded.is_unread,
    has_attachment = excluded.has_attachment,
    in_inbox = excluded.in_inbox,
    left_inbox_at = excluded.left_inbox_at,
    history_id = excluded.history_id;

-- name: DeleteThreadMessages :exec
DELETE FROM messages WHERE account_id = sqlc.arg(account_id) AND thread_id = sqlc.arg(thread_id);

-- name: InsertMessage :exec
INSERT INTO messages (account_id, id, thread_id, internal_date, from_name, from_email, snippet)
VALUES (sqlc.arg(account_id), sqlc.arg(id), sqlc.arg(thread_id), sqlc.arg(internal_date),
        sqlc.narg(from_name), sqlc.arg(from_email), sqlc.arg(snippet));

-- name: InsertMessageLabel :exec
INSERT OR IGNORE INTO message_labels (account_id, message_id, label_id)
VALUES (sqlc.arg(account_id), sqlc.arg(message_id), sqlc.arg(label_id));

-- name: GetThread :one
SELECT account_id, id, subject, snippet, last_message_at, last_from_name, last_from_email,
       message_count, is_unread, has_attachment, in_inbox
FROM threads
WHERE account_id = sqlc.arg(account_id) AND id = sqlc.arg(id);

-- name: ListThreadUserLabels :many
SELECT DISTINCT l.id, l.name, l.type, l.color_bg, l.color_fg
FROM messages m
JOIN message_labels ml ON ml.account_id = m.account_id AND ml.message_id = m.id
JOIN labels l ON l.account_id = ml.account_id AND l.id = ml.label_id
WHERE m.account_id = sqlc.arg(account_id) AND m.thread_id = sqlc.arg(thread_id) AND l.type = 'user'
ORDER BY l.name, l.id;

-- name: DeleteThread :execrows
DELETE FROM threads WHERE account_id = sqlc.arg(account_id) AND id = sqlc.arg(id);

-- name: ListInboxThreadIDs :many
SELECT id FROM threads WHERE account_id = sqlc.arg(account_id) AND in_inbox = 1;

-- name: MarkThreadLeftInbox :exec
UPDATE threads SET in_inbox = 0, left_inbox_at = sqlc.arg(left_inbox_at)
WHERE account_id = sqlc.arg(account_id) AND id = sqlc.arg(id);

-- name: DeleteThreadInboxLabels :exec
DELETE FROM message_labels
WHERE message_labels.account_id = sqlc.arg(account_id) AND label_id = 'INBOX'
  AND message_id IN (SELECT m.id FROM messages m WHERE m.account_id = sqlc.arg(account_id) AND m.thread_id = sqlc.arg(thread_id));

-- name: UnreadCounts :many
SELECT account_id, COUNT(*) AS unread
FROM threads
WHERE in_inbox = 1 AND is_unread = 1
GROUP BY account_id;

-- name: PurgeThreadsLeftInboxBefore :execrows
DELETE FROM threads WHERE in_inbox = 0 AND left_inbox_at < sqlc.arg(cutoff);
