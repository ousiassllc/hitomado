package secrets

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ousiassllc/hitomado/internal/asciijson"
)

// envelopeBridge は「メソッド名と String 引数で Java を呼び、JSON の封筒を返す」呼び出し口を Bridge にする。
// 封筒は Java の SecretBridge（WailsBridge.secureGet などと同じ形式）:
// {"ok":true,"found":bool,"value":"…"} または {"ok":false,"error":"<コード>"}。コードは Wails の SecureGet と同じく
// "secure storage: <コード>" のエラーにする（bridgeError が key_lost を ErrKeyLost に写像する）。
// WorkManager の Worker から Go を呼ぶとき（Activity が無く、Wails の Java ブリッジが無い）の資格情報の経路。
type envelopeBridge struct {
	call func(method, arg string) (string, bool)
}

// NewEnvelopeBridge は call（Java のメソッドを名前で呼ぶ。ok=false は呼び出しの失敗）を使う Bridge を返す。
func NewEnvelopeBridge(call func(method, arg string) (string, bool)) Bridge {
	return envelopeBridge{call: call}
}

var errEmptyKey = errors.New(bridgeErrPrefix + "empty key")

func (b envelopeBridge) SecureGet(key string) (string, bool, error) {
	if key == "" {
		return "", false, errEmptyKey
	}
	raw, err := b.invoke("secureGet", key)
	if err != nil {
		return "", false, err
	}
	var env struct {
		Found bool   `json:"found"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return "", false, fmt.Errorf("%smalformed response", bridgeErrPrefix)
	}
	if !env.Found {
		return "", false, nil
	}
	return env.Value, true, nil
}

func (b envelopeBridge) SecureSet(key, value string) error {
	if key == "" {
		return errEmptyKey
	}
	// 値（トークン）を含むためエラーに載せない。ASCII のみの JSON にする（JNI の NewStringUTF の制約）。
	arg, err := asciijson.Marshal(map[string]string{"key": key, "value": value})
	if err != nil {
		return fmt.Errorf("%sencode failed", bridgeErrPrefix)
	}
	_, err = b.invoke("secureSet", arg)
	return err
}

func (b envelopeBridge) SecureDelete(key string) error {
	if key == "" {
		return errEmptyKey
	}
	_, err := b.invoke("secureDelete", key)
	return err
}

// invoke は call の結果の封筒を検査し、成功なら封筒の生の文字列を返す。
func (b envelopeBridge) invoke(method, arg string) (string, error) {
	raw, ok := b.call(method, arg)
	if !ok {
		return "", errors.New(bridgeErrPrefix + "bridge call failed")
	}
	var env struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return "", errors.New(bridgeErrPrefix + "malformed response")
	}
	if !env.OK {
		return "", errors.New(bridgeErrPrefix + env.Error)
	}
	return raw, nil
}
