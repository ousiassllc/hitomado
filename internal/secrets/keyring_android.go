//go:build android

package secrets

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type androidStore struct{}

// NewKeyring は Android の暗号化ストレージ（EncryptedSharedPreferences。鍵は Android Keystore）を使う Store を返す。
// zalando/go-keyring は Android では D-Bus（dbus-launch）が無く動かないため、ビルドタグで差し替える（試作 #81）。
func NewKeyring() Store { return androidStore{} }

// Get は値を返す。無ければ ErrNotFound。
func (androidStore) Get(key string) (string, error) {
	v, found, err := application.Android.SecureGet(key)
	if err != nil {
		return "", fmt.Errorf("secrets: get %q: %w", key, err)
	}
	if !found {
		return "", ErrNotFound
	}
	return v, nil
}

// Set は値を保存する。
func (androidStore) Set(key, value string) error {
	if err := application.Android.SecureSet(key, value); err != nil {
		return fmt.Errorf("secrets: set %q: %w", key, err)
	}
	return nil
}

// Delete は値を削除する。無くてもエラーにしない。
func (androidStore) Delete(key string) error {
	if err := application.Android.SecureDelete(key); err != nil {
		return fmt.Errorf("secrets: delete %q: %w", key, err)
	}
	return nil
}
