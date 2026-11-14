-- name: ListAgentItems :many
SELECT id, account_id, thread_id, message_id, status, subject, from_label, reason_kind, reason,
       draft_id, to_json, questions_json, created_at, updated_at
FROM agent_items
ORDER BY updated_at DESC, id DESC;

-- name: GetAgentItem :one
SELECT id, account_id, thread_id, message_id, status, subject, from_label, reason_kind, reason,
       draft_id, to_json, questions_json, created_at, updated_at
FROM agent_items
WHERE id = sqlc.arg(id);

-- name: AgentItemExists :one
SELECT EXISTS (
    SELECT 1 FROM agent_items
    WHERE account_id = sqlc.arg(account_id) AND thread_id = sqlc.arg(thread_id) AND message_id = sqlc.arg(message_id)
) AS found;

-- name: InsertAgentItem :exec
INSERT INTO agent_items (id, account_id, thread_id, message_id, status, subject, from_label, reason_kind, reason,
                         draft_id, to_json, questions_json, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(account_id), sqlc.arg(thread_id), sqlc.arg(message_id), sqlc.arg(status),
        sqlc.arg(subject), sqlc.arg(from_label), sqlc.arg(reason_kind), sqlc.arg(reason),
        sqlc.arg(draft_id), sqlc.arg(to_json), sqlc.arg(questions_json),
        sqlc.arg(created_at), sqlc.arg(updated_at));

-- name: UpdateAgentItem :execrows
UPDATE agent_items
SET status = sqlc.arg(status), reason_kind = sqlc.arg(reason_kind), reason = sqlc.arg(reason),
    draft_id = sqlc.arg(draft_id), to_json = sqlc.arg(to_json), questions_json = sqlc.arg(questions_json),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: CountPendingAgentItems :one
SELECT COUNT(*) FROM agent_items WHERE status IN ('ready', 'asking');

-- name: DeleteFinishedAgentItemsBefore :execrows
DELETE FROM agent_items
WHERE status IN ('done', 'skipped', 'failed') AND updated_at < sqlc.arg(cutoff);
