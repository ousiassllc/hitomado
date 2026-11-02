// Package syncer は 60 秒間隔の history 差分同期・リセット同期・新着検出を提供する。
// ディレクトリは internal/sync、パッケージ名は標準ライブラリ sync との衝突を避けて syncer とする。
package syncer

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/store"
)

// RefreshResult は手動更新のアカウントごとの結果。
type RefreshResult struct {
	AccountID string
	Err       error // gmail.ErrNeedsReauth 等を wrap したもの。nil は成功
}

// Engine は Scheduler が実装する、サービス層から見た同期エンジン。
type Engine interface {
	// Start／Stop はアカウントごとのワーカーを起動・停止する（追加・削除に応じて呼ぶ）。
	Start(accountID string)
	Stop(accountID string)
	// Trigger は非同期に差分同期を要求する。
	Trigger(accountIDs []string)
	// Refresh は差分同期を今すぐ実行して完了を待つ。空は全アカウント。
	// 直近の手動更新から 5 秒未満の呼び出しは、直近の結果を返す（NF §4）。
	Refresh(ctx context.Context, accountIDs []string) []RefreshResult
	// FetchMore は inbox_page_token で受信箱の続きを取得して保存する（ListInbox のページング用）。
	// 末尾に達したら inbox_exhausted を立てる。added は新たに保存したスレッド数。
	FetchMore(ctx context.Context, accountID string) (added int, err error)
	// RefreshThread は当該スレッドのメタデータを再取得して DB に反映する（操作後の再計算用）。
	// Gmail 側で存在しなければ DB からも削除し found=false を返す。
	RefreshThread(ctx context.Context, accountID, threadID string) (row store.ThreadRow, found bool, err error)
	// Shutdown は全ワーカーを停止し、完了を待つ。
	Shutdown()
}
