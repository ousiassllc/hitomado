package notify

// Permission は通知の許可状態。
type Permission string

const (
	// PermissionGranted は通知を出せる（Android 13 以降の POST_NOTIFICATIONS の許可と、OS の通知設定がオン）。
	PermissionGranted Permission = "granted"
	// PermissionDenied は通知を出せない（許可されていない、または OS の設定でオフ）。
	PermissionDenied Permission = "denied"
	// PermissionUnsupported はアプリが許可を管理しない OS（Windows）。設定画面に許可の項目を出さない。
	PermissionUnsupported Permission = "unsupported"
)

// Background は OS 側のバックグラウンド同期・通知基盤（Android の WorkManager・通知チャネル・通知の許可）への指示。
// Android 以外では何もしない（NopBackground）。実装は internal/platform。
type Background interface {
	// ScheduleSync は定期同期の登録（enabled=true）と解除（false）。登録は冪等。アカウントが 0 件のときは解除する。
	ScheduleSync(enabled bool)
	// ForgetAccount はアカウント削除時に、そのアカウントの通知チャネルと表示中の通知を消す。
	ForgetAccount(accountID string)
	// Permission は現在の通知の許可状態。
	Permission() Permission
	// RequestPermission は許可のダイアログを出す（許可済み・永続的に拒否済みでは何も出ない）。結果は待たない。
	RequestPermission()
	// OpenSettings は OS の「このアプリの通知設定」を開く。
	OpenSettings() error
}

// NopBackground は何もしない Background（Windows）。許可の項目は出さない（PermissionUnsupported）。
type NopBackground struct{}

func (NopBackground) ScheduleSync(bool)      {}
func (NopBackground) ForgetAccount(string)   {}
func (NopBackground) Permission() Permission { return PermissionUnsupported }
func (NopBackground) RequestPermission()     {}
func (NopBackground) OpenSettings() error    { return nil }
