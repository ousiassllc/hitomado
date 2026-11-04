// Package store は SQLite（メタデータ・スニペットのみ。本文・添付は保存しない）への永続化を提供する。
// スキーマは ER 仕様（docs/architecture/er.md）に従い、goose マイグレーションを embed して起動時に適用する。
package store

import (
	"context"
	"errors"
	"time"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// ストアのエラー。
var (
	ErrNotFound       = errors.New("store: not found")
	ErrDuplicate      = errors.New("store: duplicate")           // accounts.email / oauth_clients.client_id の重複
	ErrOAuthClientUse = errors.New("store: oauth client in use") // 使用中の OAuth クライアントの削除
)

// アカウント状態（DB に保存する値）。
const (
	StatusActive      = "active"
	StatusNeedsReauth = "needs_reauth"
	StatusError       = "error"
)

// OAuthClient は登録済み OAuth クライアント（Secret は secrets ストア）。
type OAuthClient struct {
	ID           string // ULID
	Name         string
	ClientID     string
	AccountCount int // 一覧取得時のみ設定
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Account は接続済みアカウント。
type Account struct {
	ID            string // ULID
	OAuthClientID string // oauth_clients.id
	Email         string // 小文字
	DisplayName   string
	Color         string
	SignatureHTML string
	NotifyEnabled bool
	Status        string
	StatusMessage string
	SortOrder     int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// AccountPatch は UpdateAccount の部分更新（nil は変更なし）。
type AccountPatch struct {
	DisplayName   *string
	Color         *string
	NotifyEnabled *bool
}

// SyncState はアカウントごとの同期状態。
type SyncState struct {
	AccountID       string
	HistoryID       string // 空は未同期
	InboxPageToken  string
	InboxExhausted  bool
	InitialSyncDone bool
	LastSyncedAt    time.Time // ゼロ値は未同期
	LastError       string
}

// Label は Gmail ラベル。
type Label struct {
	ID      string
	Name    string
	Type    string // system / user
	ColorBg string
	ColorFg string
}

// MessageSnapshot は同期で得たメッセージ 1 件分のメタデータ。
type MessageSnapshot struct {
	ID           string
	InternalDate time.Time
	FromName     string
	FromEmail    string // 小文字
	Snippet      string
	LabelIDs     []string
}

// ThreadSnapshot は 1 スレッド分の反映内容。ApplyThread は messages／message_labels を丸ごと置き換え、
// threads の集計値（最新メッセージ・件数・未読・受信箱）を再計算する（1 スレッド 1 トランザクション）。
type ThreadSnapshot struct {
	AccountID     string
	ThreadID      string
	HistoryID     string
	Subject       string // 最初のメッセージの件名
	HasAttachment bool
	Messages      []MessageSnapshot // 時系列昇順
}

// ApplyResult は ApplyThread の結果。
type ApplyResult struct {
	NewMessageIDs []string // この呼び出しで初めて保存されたメッセージ
}

// ThreadRow は受信箱リスト用のスレッド行。
type ThreadRow struct {
	AccountID     string
	ThreadID      string
	Subject       string
	Snippet       string
	LastMessageAt time.Time
	LastFrom      model.Address
	MessageCount  int
	IsUnread      bool
	HasAttachment bool
	InInbox       bool
	Labels        []Label // ユーザー作成ラベルのみ
}

// InboxCursor は統合受信箱のキーセットカーソル（last_message_at DESC, account_id, id）。
type InboxCursor struct {
	LastMessageAt time.Time
	AccountID     string
	ThreadID      string
}

// InboxTab は受信箱のタブ（カテゴリ・新着）による絞り込み。空は「すべて」。
type InboxTab string

const (
	InboxTabAll        InboxTab = ""
	InboxTabPrimary    InboxTab = "primary"    // CATEGORY_PERSONAL を持つ、またはどの CATEGORY_* も持たない
	InboxTabSocial     InboxTab = "social"     // CATEGORY_SOCIAL
	InboxTabPromotions InboxTab = "promotions" // CATEGORY_PROMOTIONS
	InboxTabNew        InboxTab = "new"        // UNREAD（未読のスレッドだけ）
)

// Valid は t が定義済みのタブかを返す。
func (t InboxTab) Valid() bool {
	switch t {
	case InboxTabAll, InboxTabPrimary, InboxTabSocial, InboxTabPromotions, InboxTabNew:
		return true
	}
	return false
}

// InboxFilter は受信箱の取得条件。
type InboxFilter struct {
	AccountIDs []string // 空は全アカウント
	UnreadOnly bool
	Tab        InboxTab     // タブによる絞り込み（UnreadOnly とは AND）
	After      *InboxCursor // nil は先頭から
	Limit      int
}

// Contact は宛先候補。
type Contact struct {
	Email       string
	DisplayName string
}

// Store は永続化の抽象。実装は sqlc 生成コードを包む。
type Store interface {
	// OAuth クライアント
	ListOAuthClients(ctx context.Context) ([]OAuthClient, error) // AccountCount 付き
	GetOAuthClient(ctx context.Context, id string) (OAuthClient, error)
	CreateOAuthClient(ctx context.Context, c OAuthClient) error // client_id 重複は ErrDuplicate
	UpdateOAuthClient(ctx context.Context, id, name, clientID string) (OAuthClient, error)
	DeleteOAuthClient(ctx context.Context, id string) error // 使用中は ErrOAuthClientUse

	// アカウント
	ListAccounts(ctx context.Context) ([]Account, error) // sort_order 昇順
	GetAccount(ctx context.Context, id string) (Account, error)
	CreateAccount(ctx context.Context, a Account) error // email 重複は ErrDuplicate。account_sync_state も作成する
	UpdateAccount(ctx context.Context, id string, p AccountPatch) (Account, error)
	SetAccountStatus(ctx context.Context, id, status, message string) error
	SetSignature(ctx context.Context, id, signatureHTML string) (Account, error)
	ReorderAccounts(ctx context.Context, ids []string) error
	DeleteAccount(ctx context.Context, id string) error // 子テーブルへカスケード

	// 同期状態
	GetSyncState(ctx context.Context, accountID string) (SyncState, error)
	SaveSyncState(ctx context.Context, s SyncState) error

	// ラベル
	ReplaceLabels(ctx context.Context, accountID string, labels []Label) error
	ListLabels(ctx context.Context, accountID string) ([]Label, error)

	// スレッド
	ApplyThread(ctx context.Context, s ThreadSnapshot) (ApplyResult, error)
	// ReplaceInbox はリセット同期用。当該アカウントの in_inbox スレッドを snapshots で置き換える
	// （snapshots に無い in_inbox スレッドは受信箱から外した扱いにする）。
	ReplaceInbox(ctx context.Context, accountID string, snapshots []ThreadSnapshot) error
	DeleteThread(ctx context.Context, accountID, threadID string) error
	GetThread(ctx context.Context, accountID, threadID string) (ThreadRow, error) // 無ければ ErrNotFound
	ListInbox(ctx context.Context, f InboxFilter) ([]ThreadRow, error)
	UnreadCounts(ctx context.Context) (map[string]int, error) // in_inbox=1 AND is_unread=1
	PurgeThreadsLeftInboxBefore(ctx context.Context, cutoff time.Time) (int, error)

	// 連絡先候補（アカウントごと）
	AddReceivedContacts(ctx context.Context, accountID string, addrs []model.Address, at time.Time) error
	AddSentContacts(ctx context.Context, accountID string, addrs []model.Address, at time.Time) error
	SuggestContacts(ctx context.Context, accountID, prefix string, limit int) ([]Contact, error)

	// 設定（値は JSON 文字列）
	GetSetting(ctx context.Context, key string) (value string, ok bool, err error)
	SetSetting(ctx context.Context, key, value string) error

	Close() error
}

// 設定キー（ER §3.9）。
const (
	SettingInboxFilter = "inbox.filter"
	SettingAIConfig    = "ai.config"
	SettingAIConsent   = "ai.consent"
	SettingAIChatGPT   = "ai.chatgpt"
	SettingUpdateCheck = "update.check_enabled"
	SettingLastChecked = "update.last_checked_at"
	SettingWindowState = "window.state"
	SettingMCPConfig   = "mcp.config"
)
