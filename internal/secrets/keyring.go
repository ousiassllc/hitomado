//go:build !android

package secrets

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// keyringService は資格情報ストア上のサービス名。キー名（hitomado/<種別>/<ID>）が user になる。
const keyringService = "Hitomado"

type keyringStore struct{}

// NewKeyring は OS の資格情報ストア（Windows 資格情報マネージャー）を使う Store を返す。
// 値は分割せずそのまま保存する。OS の上限（Windows は 2.5KB 程度）を超える値は
// keyring.ErrSetDataTooBig を wrap したエラーを返す。
func NewKeyring() Store { return keyringStore{} }

// Get は値を返す。無ければ ErrNotFound。
func (keyringStore) Get(key string) (string, error) {
	v, err := keyring.Get(keyringService, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("secrets: get %q: %w", key, err)
	}
	return v, nil
}

// Set は値を保存する。
func (keyringStore) Set(key, value string) error {
	if err := keyring.Set(keyringService, key, value); err != nil {
		return fmt.Errorf("secrets: set %q: %w", key, err)
	}
	return nil
}

// Delete は値を削除する。無くてもエラーにしない。
func (keyringStore) Delete(key string) error {
	if err := keyring.Delete(keyringService, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("secrets: delete %q: %w", key, err)
	}
	return nil
}
