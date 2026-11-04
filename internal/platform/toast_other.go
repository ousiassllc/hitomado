//go:build !android

package platform

import (
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/ousiassllc/hitomado/internal/notify"
)

// toast は OS のトースト通知を出す。クリックで onResponse が呼ばれる。
func (n *Notifier) toast(m notify.NewMailNotice, title, body string) error {
	return n.svc.SendNotification(notifications.NotificationOptions{
		ID:    m.AccountID + ":" + m.ThreadID,
		Title: title,
		Body:  body,
		Data:  map[string]any{"accountId": m.AccountID, "threadId": m.ThreadID},
	})
}
