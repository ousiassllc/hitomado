package platform

import (
	appevents "github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/notify"
)

// deliverOpen は準備済みのフロントへ通知のタップの要求（開くスレッド）を notify:open-thread で届ける。
func deliverOpen(emit appevents.Emitter) func(notify.OpenRef) {
	return func(r notify.OpenRef) {
		emit.Emit(appevents.NotifyOpenThread, appevents.OpenThreadPayload{AccountID: r.AccountID, ThreadID: r.ThreadID})
	}
}
