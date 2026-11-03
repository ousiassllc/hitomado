package notify

import (
	"log/slog"
)

// JavaBackground は Java 側（WailsBridge の notification 系メソッド）を呼んで Background を実現する（Android）。
// Call は Java のメソッドを名前と JSON のペイロードで呼び、{"ok":true,"value":"…"} の value を返す
// （internal/platform の callJava）。Java との接続（JNI）と分けてあるので、純 Go でテストできる。
type JavaBackground struct {
	Call   func(method string, payload any) (string, error)
	Logger *slog.Logger // nil は破棄
}

var _ Background = JavaBackground{}

// Java 側のメソッド名（WailsBridge.java）。
const (
	methodScheduleSync      = "scheduleSync"
	methodForgetAccount     = "forgetAccount"
	methodPermission        = "notificationPermission"
	methodRequestPermission = "requestNotificationPermission"
	methodOpenSettings      = "openNotificationSettings"
)

func (b JavaBackground) log() *slog.Logger {
	if b.Logger != nil {
		return b.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// ScheduleSync は WorkManager の定期同期を登録・解除する。失敗しても呼び出し側（アカウント追加など）は止めない。
func (b JavaBackground) ScheduleSync(enabled bool) {
	if _, err := b.Call(methodScheduleSync, map[string]any{"enabled": enabled}); err != nil {
		b.log().Warn("schedule background sync failed", "enabled", enabled, "err", err)
	}
}

// ForgetAccount はアカウントの通知チャネルと表示中の通知を消す。
func (b JavaBackground) ForgetAccount(accountID string) {
	if _, err := b.Call(methodForgetAccount, map[string]any{"accountId": accountID}); err != nil {
		b.log().Warn("forget account notifications failed", "account", accountID, "err", err)
	}
}

// Permission は Java の通知の許可状態。呼び出しに失敗したときは PermissionUnsupported（項目を出さない）。
func (b JavaBackground) Permission() Permission {
	v, err := b.Call(methodPermission, map[string]any{})
	if err != nil {
		b.log().Warn("notification permission failed", "err", err)
		return PermissionUnsupported
	}
	switch Permission(v) {
	case PermissionGranted:
		return PermissionGranted
	case PermissionDenied:
		return PermissionDenied
	}
	return PermissionUnsupported
}

// RequestPermission は許可のダイアログを出す（結果は待たない）。
func (b JavaBackground) RequestPermission() {
	if _, err := b.Call(methodRequestPermission, map[string]any{}); err != nil {
		b.log().Warn("request notification permission failed", "err", err)
	}
}

// OpenSettings は OS の「このアプリの通知設定」を開く。
func (b JavaBackground) OpenSettings() error {
	if _, err := b.Call(methodOpenSettings, map[string]any{}); err != nil {
		return err
	}
	return nil
}
