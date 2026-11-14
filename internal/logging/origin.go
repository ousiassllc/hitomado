package logging

import "log/slog"

// OriginKey はログのどの実行経路（アプリ本体か、バックグラウンド同期か）が出したかを示す属性名。
// 「再接続が必要になった」など、経路によって原因の見立てが変わるログを読み分けるために使う。
const OriginKey = "origin"

const (
	// OriginApp はアプリ本体（Wails の画面・前面の 60 秒ループ・WorkManager から頼まれたアプリ本体の更新）。
	OriginApp = "app"
	// OriginAppBackground は、WorkManager の Worker に頼まれて、アプリ本体のあるプロセスのスケジューラが行った更新（バックグラウンド同期）。
	OriginAppBackground = "app_background"
	// OriginHeadless は Activity の無い WorkManager の Worker が動かすヘッドレス同期（Android。internal/headless）。
	OriginHeadless = "headless"
)

// WithOrigin は l の出力すべてに origin 属性を付けた logger を返す。
func WithOrigin(l *slog.Logger, origin string) *slog.Logger {
	return l.With(OriginKey, origin)
}
