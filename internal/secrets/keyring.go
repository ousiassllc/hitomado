//go:build !android

package secrets

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	// keyringService は資格情報ストア上のサービス名。キー名（nefos-inbox/<種別>/<ID>）が user になる。
	keyringService = "Nefos Inbox"
	// legacyKeyringService は v0.1.46 までのサービス名（キー名は hitomado/<種別>/<ID>）。読み出し時に keyringService へ移す。
	legacyKeyringService = "Hitomado"
)

type keyringStore struct{ service string }

// NewKeyring は OS の資格情報ストア（Windows 資格情報マネージャー）を使う Store を返す。
// 値は分割せずそのまま保存する。OS の上限（Windows は 2.5KB 程度）を超える値は
// keyring.ErrSetDataTooBig を wrap したエラーを返す。
// 旧名（Hitomado）で保存された資格情報は、読んだときに新しい名前へ移す（WithLegacy）。再ログインは要らない。
func NewKeyring() Store {
	return WithLegacy(keyringStore{service: keyringService}, keyringStore{service: legacyKeyringService})
}

// Get は値を返す。無ければ ErrNotFound。
func (k keyringStore) Get(key string) (string, error) {
	v, err := keyring.Get(k.service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("secrets: get %q: %w", key, err)
	}
	return v, nil
}

// Set は値を保存する。
func (k keyringStore) Set(key, value string) error {
	if err := keyring.Set(k.service, key, value); err != nil {
		return fmt.Errorf("secrets: set %q: %w", key, err)
	}
	return nil
}

// Delete は値を削除する。無くてもエラーにしない。
func (k keyringStore) Delete(key string) error {
	if err := keyring.Delete(k.service, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("secrets: delete %q: %w", key, err)
	}
	return nil
}
