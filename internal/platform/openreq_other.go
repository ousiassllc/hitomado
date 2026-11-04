//go:build !android

package platform

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	appevents "github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/notify"
)

// NewOpenRequests は通知のタップで開くスレッドの受け渡し口を返す。Windows のトーストのクリックは Notifier.onResponse が
// 直接 notify:open-thread を発行するため、Java からの要求は無い（pull なし）。
func NewOpenRequests(_ *application.App, emit appevents.Emitter) *notify.OpenRequests {
	return notify.NewOpenRequests(deliverOpen(emit), nil)
}

// NewBackground はバックグラウンド同期・通知の許可の窓口を返す。Windows では何もしない。
func NewBackground(_ *slog.Logger) notify.Background { return notify.NopBackground{} }
