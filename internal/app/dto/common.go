// Package dto はバインディング（Go↔フロント）で受け渡す型を定義する（docs/api/endpoints.md §1.3〜§7）。
// JSON は camelCase、日時は Unix ミリ秒、ID は文字列。
package dto

import "github.com/ousiassllc/hitomado/internal/domain/model"

// ThreadRef はスレッドの参照。
type ThreadRef struct {
	AccountID string `json:"accountId"`
	ThreadID  string `json:"threadId"`
}

// MessageRef はメッセージの参照。
type MessageRef struct {
	AccountID string `json:"accountId"`
	ThreadID  string `json:"threadId"`
	MessageID string `json:"messageId"`
}

// Account は接続済みアカウント。Email は返信・転送の送信元（変更不可）。
type Account struct {
	ID            string `json:"id"`
	OAuthClientID string `json:"oauthClientId"`
	Email         string `json:"email"`
	DisplayName   string `json:"displayName"`
	Color         string `json:"color"` // パレットキー
	SignatureHTML string `json:"signatureHtml"`
	NotifyEnabled bool   `json:"notifyEnabled"`
	Status        string `json:"status"` // active / needs_reauth / error
	StatusMessage string `json:"statusMessage,omitempty"`
	SortOrder     int    `json:"sortOrder"`
}

// GmailSignature は Gmail の設定から取得した署名（F-07。#110）。保存はしない。
type GmailSignature struct {
	SignatureHTML string `json:"signatureHtml"` // 許可タグへ整形済み。Found が偽なら ""
	Found         bool   `json:"found"`         // Gmail に署名が設定されているか
	Sanitized     bool   `json:"sanitized"`     // 整形で内容（画像・装飾など）を落としたか
}

// OAuthClient は登録済み OAuth クライアント（Secret は返さない）。
type OAuthClient struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ClientID     string `json:"clientId"`
	AccountCount int    `json:"accountCount"`
	HasSecret    bool   `json:"hasSecret"`
}

// Label はユーザー作成ラベル。
type Label struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	ColorBg string `json:"colorBg,omitempty"`
	ColorFg string `json:"colorFg,omitempty"`
}

// ThreadSummary は受信箱・検索結果の 1 行。
type ThreadSummary struct {
	AccountID     string        `json:"accountId"`
	ThreadID      string        `json:"threadId"`
	Subject       string        `json:"subject"`
	Snippet       string        `json:"snippet"`
	LastMessageAt int64         `json:"lastMessageAt"`
	LastFrom      model.Address `json:"lastFrom"`
	MessageCount  int           `json:"messageCount"`
	IsUnread      bool          `json:"isUnread"`
	HasAttachment bool          `json:"hasAttachment"`
	InInbox       bool          `json:"inInbox"`
	Labels        []Label       `json:"labels"`
}

// AccountFailure は一部アカウントの取得失敗。Error は apperr.Error の JSON 形（code/message/retryable…）。
type AccountFailure struct {
	AccountID string         `json:"accountId"`
	Error     map[string]any `json:"error"`
}

// ThreadPage はページング付きのスレッド一覧。NextCursor が空なら末尾。
type ThreadPage struct {
	Items      []ThreadSummary  `json:"items"`
	NextCursor string           `json:"nextCursor,omitempty"`
	Failures   []AccountFailure `json:"failures"`
}
