package app

import (
	"context"
	"time"

	"github.com/ousiassllc/hitomado/internal/events"
)

// updateCheckInterval は定期的な新バージョン確認の間隔（NF-D03）。
const updateCheckInterval = 24 * time.Hour

// UpdateLoop は新バージョンの定期確認。Wails のバインディングには登録しない（main が goroutine で Run を呼ぶ）。
type UpdateLoop struct {
	d   *Deps
	app *AppService
}

// NewUpdateLoop は UpdateLoop を作る。
func NewUpdateLoop(d *Deps) *UpdateLoop { return &UpdateLoop{d: d, app: NewAppService(d)} }

// Run は起動時と 24 時間ごとに（有効なときだけ）更新を確認し、新版なら update:available を発行する。
// ctx のキャンセルまでブロックする。失敗は記録して次の周期に任せる。
func (l *UpdateLoop) Run(ctx context.Context) {
	var tick <-chan time.Time
	if l.d.UpdateTicker != nil {
		var stop func()
		tick, stop = l.d.UpdateTicker()
		defer stop()
	} else {
		t := time.NewTicker(updateCheckInterval)
		defer t.Stop()
		tick = t.C
	}
	for {
		l.checkAndEmit(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
	}
}

func (l *UpdateLoop) checkAndEmit(ctx context.Context) {
	info, err := l.app.CheckForUpdate(ctx)
	if err != nil {
		if ctx.Err() == nil {
			logger(l.d).Info("更新確認に失敗", "err", err)
		}
		return
	}
	if info.Available {
		l.d.Events.Emit(events.UpdateAvailable, events.UpdateAvailablePayload{Version: info.Version, URL: info.URL})
	}
}
