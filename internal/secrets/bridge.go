package secrets

import (
	"fmt"
	"strings"
)

// 以下は Android の暗号化ストレージ（Java の SecureStore。EncryptedSharedPreferences）を Store にする部分。
// Java 側の呼び出し（JNI）と分けてあり、build tag なしの純 Go なので、Windows の CI でもユニットテストできる
// （実際の接続は keyring_android.go）。

// bridgeErrPrefix は Wails の SecureGet/SecureSet/SecureDelete が Java 側の失敗に付ける接頭辞。
// 残りの部分は Java の SecureStore が返すコード（key_lost／unavailable／failed）で、値やキーの中身は含まれない。
const bridgeErrPrefix = "secure storage: "

// Java の SecureStore.Kind.KEY_LOST のコードと一致させる（build/android/.../SecureStore.java）。
const bridgeCodeKeyLost = "key_lost" // Keystore の鍵が失われ、ストアを作り直した

// bridge は Wails の application.Android のうち、資格情報に使う 3 つのメソッド。
type bridge interface {
	SecureGet(key string) (value string, found bool, err error)
	SecureSet(key, value string) error
	SecureDelete(key string) error
}

// bridgeStore は bridge を使う Store。Java 側が暗号化・Keystore・排他を担う。
type bridgeStore struct{ b bridge }

func newBridgeStore(b bridge) Store { return bridgeStore{b: b} }

// Get は値を返す。無ければ ErrNotFound。Keystore の鍵が失われていれば ErrKeyLost（errors.Is(ErrNotFound) も真）。
func (s bridgeStore) Get(key string) (string, error) {
	v, found, err := s.b.SecureGet(key)
	if err != nil {
		return "", bridgeError("get", key, err)
	}
	if !found {
		return "", ErrNotFound
	}
	return v, nil
}

// Set は値を保存する。鍵が失われていた場合は、Java 側がストアを作り直してから保存する。
func (s bridgeStore) Set(key, value string) error {
	if err := s.b.SecureSet(key, value); err != nil {
		return bridgeError("set", key, err)
	}
	return nil
}

// Delete は値を削除する。無くてもエラーにしない。
func (s bridgeStore) Delete(key string) error {
	if err := s.b.SecureDelete(key); err != nil {
		return bridgeError("delete", key, err)
	}
	return nil
}

// bridgeError は Java 側の失敗を Store のエラーにする。鍵の喪失は ErrKeyLost にし、それ以外は元のエラーを wrap する。
// 値は引数に取らない（エラー文字列に値が入らないようにする）。
func bridgeError(op, key string, err error) error {
	if strings.TrimPrefix(err.Error(), bridgeErrPrefix) == bridgeCodeKeyLost {
		return ErrKeyLost
	}
	return fmt.Errorf("secrets: %s %q: %w", op, key, err)
}
