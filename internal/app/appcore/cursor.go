// Package appcore は受信箱・検索・スレッド表示の純粋ロジック（カーソル・並列実行・本文 HTML 加工・ファイル名正規化）を持つ。
// バインディングのサービス（internal/app）から使う。
package appcore

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/store"
)

var errBadCursor = apperr.New(apperr.Validation, "cursor が正しくありません。")

// inboxCursorJSON はキーセットカーソルの中身。クライアントには base64url の不透明文字列として渡す。
type inboxCursorJSON struct {
	T int64  `json:"t"` // last_message_at（Unix ミリ秒）
	A string `json:"a"`
	I string `json:"i"`
}

// EncodeInboxCursor は受信箱のキーセットカーソルを不透明文字列にする。
func EncodeInboxCursor(c store.InboxCursor) string {
	b, err := json.Marshal(inboxCursorJSON{T: c.LastMessageAt.UnixMilli(), A: c.AccountID, I: c.ThreadID})
	if err != nil {
		return "" // 文字列と整数のみのため発生しない
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeInboxCursor は空文字なら nil を返す。不正なカーソルは VALIDATION。
func DecodeInboxCursor(s string) (*store.InboxCursor, error) {
	if s == "" {
		return nil, nil
	}
	var c inboxCursorJSON
	if err := decodeCursor(s, &c); err != nil || c.A == "" || c.I == "" {
		return nil, errBadCursor
	}
	return &store.InboxCursor{LastMessageAt: time.UnixMilli(c.T), AccountID: c.A, ThreadID: c.I}, nil
}

func decodeCursor(s string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// RestEntry は一度 threads.list で得たが、まだ配信していないスレッド。時刻を持つのでマージ時に再取得しない。
type RestEntry struct {
	I string `json:"i"` // threadId
	T int64  `json:"t"` // 最終メッセージ日時（Unix ミリ秒）
}

// SearchState はアカウント 1 つ分の検索の続き: 次の pageToken と、配信前の残り。
type SearchState struct {
	PT   string      `json:"p,omitempty"`
	Rest []RestEntry `json:"r,omitempty"`
}

func (st SearchState) empty() bool { return st.PT == "" && len(st.Rest) == 0 }

// searchCursorJSON は検索カーソルの中身（サーバー状態を持たない設計。アカウントごとの続きをすべて埋め込む）。
type searchCursorJSON struct {
	V int                    `json:"v"`
	A map[string]SearchState `json:"a"`
}

// EncodeSearchCursor はアカウントごとの検索の続きを不透明文字列にする。すべて尽きていれば空文字。
func EncodeSearchCursor(states map[string]SearchState) string {
	c := searchCursorJSON{V: 1, A: map[string]SearchState{}}
	for id, st := range states {
		if !st.empty() {
			c.A[id] = st
		}
	}
	if len(c.A) == 0 {
		return ""
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "" // 文字列と整数のみのため発生しない
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeSearchCursor は空文字なら nil を返す。不正なカーソルは VALIDATION。
func DecodeSearchCursor(s string) (map[string]SearchState, error) {
	if s == "" {
		return nil, nil
	}
	var c searchCursorJSON
	if err := decodeCursor(s, &c); err != nil || c.V != 1 || len(c.A) == 0 {
		return nil, errBadCursor
	}
	return c.A, nil
}
