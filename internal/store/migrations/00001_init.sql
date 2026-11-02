-- +goose Up
-- docs/architecture/er.md のスキーマ。日時は UTC の Unix ミリ秒、真偽値は 0/1。
-- メッセージ本文・添付・下書きを保存するカラム・テーブルは作らない（NF-S08）。

CREATE TABLE oauth_clients (
    id         TEXT    NOT NULL PRIMARY KEY,
    name       TEXT    NOT NULL CHECK (length(name) > 0),
    client_id  TEXT    NOT NULL UNIQUE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE accounts (
    id              TEXT    NOT NULL PRIMARY KEY,
    oauth_client_id TEXT    NOT NULL REFERENCES oauth_clients (id) ON DELETE RESTRICT,
    email           TEXT    NOT NULL UNIQUE,
    display_name    TEXT    NOT NULL,
    color           TEXT    NOT NULL,
    signature_html  TEXT    NOT NULL DEFAULT '',
    notify_enabled  INTEGER NOT NULL DEFAULT 1 CHECK (notify_enabled IN (0, 1)),
    status          TEXT    NOT NULL CHECK (status IN ('active', 'needs_reauth', 'error')),
    status_message  TEXT,
    sort_order      INTEGER NOT NULL,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

CREATE TABLE account_sync_state (
    account_id        TEXT    NOT NULL PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    history_id        TEXT,
    inbox_page_token  TEXT,
    inbox_exhausted   INTEGER NOT NULL DEFAULT 0 CHECK (inbox_exhausted IN (0, 1)),
    initial_sync_done INTEGER NOT NULL DEFAULT 0 CHECK (initial_sync_done IN (0, 1)),
    last_synced_at    INTEGER,
    last_error        TEXT
);

CREATE TABLE labels (
    account_id TEXT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    id         TEXT NOT NULL,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL CHECK (type IN ('system', 'user')),
    color_bg   TEXT,
    color_fg   TEXT,
    PRIMARY KEY (account_id, id)
);

CREATE TABLE threads (
    account_id      TEXT    NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    id              TEXT    NOT NULL,
    subject         TEXT    NOT NULL,
    snippet         TEXT    NOT NULL,
    last_message_at INTEGER NOT NULL,
    last_from_name  TEXT,
    last_from_email TEXT    NOT NULL,
    message_count   INTEGER NOT NULL,
    is_unread       INTEGER NOT NULL CHECK (is_unread IN (0, 1)),
    has_attachment  INTEGER NOT NULL CHECK (has_attachment IN (0, 1)),
    in_inbox        INTEGER NOT NULL CHECK (in_inbox IN (0, 1)),
    left_inbox_at   INTEGER,
    history_id      TEXT    NOT NULL,
    PRIMARY KEY (account_id, id)
);

CREATE INDEX idx_threads_inbox ON threads (last_message_at DESC, account_id, id) WHERE in_inbox = 1;
CREATE INDEX idx_threads_unread ON threads (account_id) WHERE in_inbox = 1 AND is_unread = 1;
CREATE INDEX idx_threads_retention ON threads (left_inbox_at) WHERE in_inbox = 0;

CREATE TABLE messages (
    account_id    TEXT    NOT NULL,
    id            TEXT    NOT NULL,
    thread_id     TEXT    NOT NULL,
    internal_date INTEGER NOT NULL,
    from_name     TEXT,
    from_email    TEXT    NOT NULL,
    snippet       TEXT    NOT NULL,
    PRIMARY KEY (account_id, id),
    FOREIGN KEY (account_id, thread_id) REFERENCES threads (account_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_messages_thread ON messages (account_id, thread_id, internal_date);

-- label_id は labels を論理参照する（外部キーは張らない）。
CREATE TABLE message_labels (
    account_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    label_id   TEXT NOT NULL,
    PRIMARY KEY (account_id, message_id, label_id),
    FOREIGN KEY (account_id, message_id) REFERENCES messages (account_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_message_labels_label ON message_labels (account_id, label_id, message_id);

CREATE TABLE contacts (
    account_id    TEXT    NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    email         TEXT    NOT NULL,
    display_name  TEXT,
    send_count    INTEGER NOT NULL DEFAULT 0,
    receive_count INTEGER NOT NULL DEFAULT 0,
    last_used_at  INTEGER NOT NULL,
    PRIMARY KEY (account_id, email)
);

CREATE INDEX idx_contacts_rank ON contacts (account_id, last_used_at DESC);

CREATE TABLE app_settings (
    key        TEXT    NOT NULL PRIMARY KEY,
    value      TEXT    NOT NULL CHECK (json_valid(value)),
    updated_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE app_settings;
DROP TABLE contacts;
DROP TABLE message_labels;
DROP TABLE messages;
DROP TABLE threads;
DROP TABLE labels;
DROP TABLE account_sync_state;
DROP TABLE accounts;
DROP TABLE oauth_clients;
