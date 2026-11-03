//go:build !android

package platform

import (
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// toast は OS のトースト通知を出す。クリックで onResponse が呼ばれる。
func (n *Notifier) toast(accountID, threadID, title, body string) error {
	return n.svc.SendNotification(notifications.NotificationOptions{
		ID:    accountID + ":" + threadID,
		Title: title,
		Body:  body,
		Data:  map[string]any{"accountId": accountID, "threadId": threadID},
	})
}
