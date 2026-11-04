// Package notify は新着通知とトレイバッジの抽象を提供する。Wails 依存の実装は internal/platform に置く。
package notify

import "context"

// NewMailNotice は新着 1 件分の通知内容。
type NewMailNotice struct {
	AccountID    string
	AccountName  string // 表示名（空ならアドレス）
	AccountEmail string // アカウントのアドレス（Android の通知チャネルの表示名に使う）
	ThreadID     string
	FromName     string // 空ならアドレス
	Subject      string
}

// Notifier はトースト通知とトレイアイコンのバッジ。
type Notifier interface {
	// NewMail はトーストを出す。クリックで notify:open-thread を発行し、ウィンドウを前面に出す。
	NewMail(ctx context.Context, n NewMailNotice) error
	// SetUnread は全アカウント合計の未読有無をトレイアイコンに反映する。
	SetUnread(hasUnread bool)
}
