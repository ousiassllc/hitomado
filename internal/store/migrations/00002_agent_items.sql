-- +goose Up
-- エージェントモード（#372）が扱った新着 1 件。件名・差出人の表示名・下書きの ID・質問を端末内にだけ保存する。
-- メッセージ本文・下書きの本文（冒頭のプレビューを含む）・利用者の回答は保存しない（下書きの本文は Gmail 側が正で、プレビューはメモリだけで持つ）。
-- (account_id, thread_id, message_id) が一意キー（同じ新着を二度扱わない）。アカウントを削除したら行も消える。

CREATE TABLE agent_items (
    id             TEXT    NOT NULL PRIMARY KEY,
    account_id     TEXT    NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    thread_id      TEXT    NOT NULL,
    message_id     TEXT    NOT NULL,
    status         TEXT    NOT NULL CHECK (status IN ('ready', 'asking', 'skipped', 'later', 'done', 'failed')),
    subject        TEXT    NOT NULL,
    from_label     TEXT    NOT NULL,
    reason_kind    TEXT    NOT NULL,
    reason         TEXT    NOT NULL,
    draft_id       TEXT    NOT NULL DEFAULT '',
    to_json        TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(to_json)),
    questions_json TEXT    NOT NULL DEFAULT '[]' CHECK (json_valid(questions_json)),
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL,
    UNIQUE (account_id, thread_id, message_id)
);

CREATE INDEX idx_agent_items_updated ON agent_items (updated_at DESC);

-- +goose Down
DROP TABLE agent_items;
