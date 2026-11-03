//go:build !android

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// notificationService は Wails の通知サービスを登録対象として返す。
func notificationService(n *notifications.NotificationService) (application.Service, bool) {
	return application.NewService(n), true
}
