package mcpserver

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// Backend は MCP のツールが使うアプリの機能。読み取りと返信下書きの作成だけを持つ。
// 送信（PrepareSend／Send）に当たるメソッドは意図して持たない: 送信は必ず人がアプリで押す。
type Backend interface {
	Status(ctx context.Context) (StatusInfo, error)
	ListAccounts(ctx context.Context) ([]AccountInfo, error)
	ListThreads(ctx context.Context, q dto.InboxQuery) (dto.ThreadPage, error)
	SearchThreads(ctx context.Context, q dto.SearchQuery) (dto.ThreadPage, error)
	GetThread(ctx context.Context, accountID, threadID string) (ThreadView, error)
	CreateReplyDraft(ctx context.Context, req ReplyDraftRequest) (ReplyDraft, error)
}

// AccountInfo は MCP に出すアカウント情報（署名・OAuth クライアントなどは含めない）。
type AccountInfo struct {
	ID            string `json:"accountId"`
	Email         string `json:"email"`
	DisplayName   string `json:"displayName"`
	Status        string `json:"status"` // active / needs_reauth / error
	StatusMessage string `json:"statusMessage,omitempty"`
}

// AccountSync はアカウントごとの同期状態。時刻は RFC 3339。
type AccountSync struct {
	AccountInfo
	InitialSyncDone bool   `json:"initialSyncDone"`
	LastSyncedAt    string `json:"lastSyncedAt,omitempty"`
	LastError       string `json:"lastError,omitempty"`
}

// StatusInfo はアプリの状態。
type StatusInfo struct {
	Version  string        `json:"version"`
	Accounts []AccountSync `json:"accounts"`
}

// ThreadMessageView はスレッド内のメッセージ。本文はプレーンテキスト（HTML のみのメールは変換する）。
type ThreadMessageView struct {
	ID          string                 `json:"messageId"`
	From        model.Address          `json:"from"`
	To          []model.Address        `json:"to"`
	Cc          []model.Address        `json:"cc"`
	Date        string                 `json:"date"` // RFC 3339
	IsUnread    bool                   `json:"isUnread"`
	BodyText    string                 `json:"bodyText"`
	Truncated   bool                   `json:"bodyTruncated,omitempty"`
	Attachments []model.AttachmentInfo `json:"attachments"` // 一覧と情報だけ。中身は返さない
}

// ThreadView は get_thread の結果。
type ThreadView struct {
	AccountID string              `json:"accountId"`
	ThreadID  string              `json:"threadId"`
	Subject   string              `json:"subject"`
	Messages  []ThreadMessageView `json:"messages"`
	DraftID   string              `json:"draftId,omitempty"`
}

// ReplyDraftRequest は返信下書きの作成要求。送信元は ThreadID が属する（受信した）アカウントに固定される。
type ReplyDraftRequest struct {
	AccountID       string
	ThreadID        string
	SourceMessageID string // 空なら自分以外の最新メッセージ
	ReplyAll        bool
	BodyText        string // 返信の本文（署名・引用はアプリが付ける）
}

// ReplyDraft は作成した下書き。
type ReplyDraft struct {
	DraftID   string          `json:"draftId"`
	AccountID string          `json:"accountId"`
	ThreadID  string          `json:"threadId"`
	From      string          `json:"from"`
	To        []model.Address `json:"to"`
	Cc        []model.Address `json:"cc"`
	Subject   string          `json:"subject"`
}
