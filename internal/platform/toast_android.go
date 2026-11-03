//go:build android

package platform

import (
	"encoding/json"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// toast は Android の通知（NotificationManager）を出す。試作（#81）。
// Wails 同梱の Java ブリッジの通知はタップ時のデータを持たない（スレッドを開けない）ため、本実装ではブリッジの拡張が要る。
func (n *Notifier) toast(_, _, title, body string) error {
	payload, err := json.Marshal(map[string]any{"title": title, "body": body})
	if err != nil {
		return err
	}
	application.Android.Notify(string(payload))
	return nil
}
