package platform

import (
	"log/slog"
	"time"

	"github.com/ousiassllc/hitomado/internal/logging"
)

// ShutdownGrace は終了処理（同期の停止）を待つ上限。Run がトレイ終了を経由せず戻ったとき（SIGINT・セッション終了など）、
// flush 中の tray.SetIcon（InvokeSync）が戻らないと sched.Shutdown が待ち続けうるため、上限を付けて先へ進む。
const ShutdownGrace = 5 * time.Second

// WaitBounded は fn を別 goroutine で実行し、timeout 以内に戻れば true、過ぎたら待たずに false を返す。
// 時間切れのとき fn は止められない: 放棄された呼び出しは goroutine の中で後から終わる（戻らなければプロセス終了まで残る）。
// 結果は呼び出し元へ返さないので、放棄された fn が後で戻っても何も起きない。
// fn は別 goroutine で動き、呼び出し元の recover は届かない。fn の panic は log に記録して握りつぶし、戻ったものとして true を返す。
func WaitBounded(log *slog.Logger, fn func(), timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer logging.Recover(log, "bounded call")
		fn()
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}
