package updatecore

import (
	"context"
	"time"

	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/events"
)

// checkInterval は定期的な新バージョン確認の間隔（NF-D03）。
const checkInterval = 24 * time.Hour

// Loop は新バージョンの定期確認。Wails のバインディングには登録しない（main が goroutine で Run を呼ぶ）。
type Loop struct {
	d   *deps.Deps
	svc *Service
}

// NewLoop は Loop を作る。
func NewLoop(d *deps.Deps) *Loop { return &Loop{d: d, svc: New(d)} }

// Run は起動時と 24 時間ごとに（有効なときだけ）更新を確認し、新版なら update:available を発行する
// （自動アップデートが有効なら、確認の中でダウンロードも始まり、進みは update:status で届く）。
// ctx のキャンセルまでブロックする。失敗は記録して次の周期に任せる。
func (l *Loop) Run(ctx context.Context) {
	var tick <-chan time.Time
	if l.d.UpdateTicker != nil {
		var stop func()
		tick, stop = l.d.UpdateTicker()
		defer stop()
	} else {
		t := time.NewTicker(checkInterval)
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

func (l *Loop) checkAndEmit(ctx context.Context) {
	info, err := l.svc.Check(ctx)
	if err != nil {
		if ctx.Err() == nil {
			l.svc.log().Info("更新確認に失敗", "err", err)
		}
		return
	}
	if info.Available {
		l.d.Events.Emit(events.UpdateAvailable, events.UpdateAvailablePayload{Version: info.Version, URL: info.URL})
	}
}
