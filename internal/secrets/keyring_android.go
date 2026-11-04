//go:build android

package secrets

import "github.com/wailsapp/wails/v3/pkg/application"

// NewKeyring は Android の暗号化ストレージ（EncryptedSharedPreferences。鍵は Android Keystore）を使う Store を返す。
// zalando/go-keyring は Android では D-Bus（dbus-launch）が無く動かないため、ビルドタグで差し替える。
// 保存・暗号化・Keystore の鍵の喪失からの復旧は Java の SecureStore（Activity に依存せず、Worker からも使える）が担い、
// Go 側のエラーの扱いは bridge.go（純 Go）にある。
func NewKeyring() Store { return newBridgeStore(application.Android) }
