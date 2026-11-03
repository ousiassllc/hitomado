//go:build android

package platform

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	appevents "github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/notify"
)

// openThreadEvent は Java（MainActivity が通知のタップの Intent を受けたとき）が Go へ送る合図のイベント名。
// 中身は無く、要求そのものは Java が保持し、Go が takeOpenRequest で取り出す（Go の起動前に送ったイベントは取りこぼされるため）。
const openThreadEvent = "hitomado:open-thread"

// NewOpenRequests は通知のタップで開くスレッドの受け渡し口を返し、Java の合図（hitomado:open-thread）を Wake へつなぐ。
func NewOpenRequests(app *application.App, emit appevents.Emitter) *notify.OpenRequests {
	r := notify.NewOpenRequests(deliverOpen(emit), pullJavaOpen)
	app.Event.On(openThreadEvent, func(*application.CustomEvent) { r.Wake() })
	return r
}

// pullJavaOpen は Java が保持する「通知のタップで開くスレッド」を取り出す（取り出すと Java 側は空になる）。
func pullJavaOpen() *notify.OpenRef {
	v, err := callJava("takeOpenRequest", struct{}{})
	if err != nil {
		return nil
	}
	return notify.ParseOpenRef(v)
}

// NewBackground は WorkManager の定期同期・通知チャネル・通知の許可を Java に頼む窓口を返す。
func NewBackground(log *slog.Logger) notify.Background {
	return notify.JavaBackground{Call: callJava, Logger: log}
}
