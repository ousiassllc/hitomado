//go:build android

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// notificationService は Android では何も登録しない。Wails の通知サービスは Android 用の実装が無く Linux 版
// （D-Bus）が使われ、起動時に dbus-launch を探して失敗し、アプリが起動しない（試作 #81 で確認）。
// 通知は platform.Notifier が Android.Notify（Java ブリッジ）で出す。
func notificationService(*notifications.NotificationService) (application.Service, bool) {
	return application.Service{}, false
}
