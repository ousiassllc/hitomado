//go:build android

package platform

import (
	"github.com/ousiassllc/hitomado/internal/notify"
)

// toast は Android の通知（NotificationManager）を Java（WailsBridge.notifyNewMail → NewMailNotifier）で出す。
// アプリのプロセスが動いている間（前面の 60 秒ループ）の新着。バックグラウンドの同期（WorkManager の Worker）の新着も、
// 同じ NewMailNotifier で出す（internal/headless が返した新着を Java が通知する）ため、チャネル・グループ・タップの動きは同じ。
// Wails 同梱の Notify（タップ時のデータを持たず、スレッドを開けない）は使わない。
// 見出し・本文は notify.Text の整形済み（Windows と同じ F-10）。ロック画面で本文を隠すのは Java 側（VISIBILITY_PRIVATE）。
func (n *Notifier) toast(m notify.NewMailNotice, title, body string) error {
	_, err := callJava("notifyNewMail", map[string]any{
		"accountId":    m.AccountID,
		"accountEmail": notify.PlainText(m.AccountEmail),
		"threadId":     m.ThreadID,
		"title":        title,
		"body":         body,
	})
	return err
}
