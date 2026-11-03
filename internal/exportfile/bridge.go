package exportfile

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"
)

// 以下は Java（build/android/.../DocumentExporter.java）との間でやり取りする JSON。JNI を使う呼び出し自体は
// platform の *_android.go にある。

// EventName は Java が選択・保存の結果を Go へ知らせるイベント名（WailsBridge.emitEvent → application の Event）。
const EventName = "hitomado:document"

// Status は Java が知らせる結果の種類。
type Status string

// Java の DocumentExporter の STATUS_* と一致させる。
const (
	StatusPicked    Status = "picked"    // 保存先が選ばれた
	StatusCancelled Status = "cancelled" // 利用者が保存先の選択をキャンセルした
	StatusSaved     Status = "saved"     // 一時ファイルから保存先へのコピーが終わった
	StatusFailed    Status = "failed"    // 失敗（Error にコード）
)

// Result は EventName で届く 1 件の結果。
type Result struct {
	ID     int
	Status Status
	Name   string // Saved: 保存先の表示名（Java が ContentResolver から取る。取れなければ空）
	Error  string // Failed: Java 側のコード（失敗の理由。パス・Uri を含まない）
}

// ParseResult は Wails のイベントの Data（JSON を map にしたもの）を Result にする。形が違えば ok=false。
func ParseResult(data any) (Result, bool) {
	m, ok := data.(map[string]any)
	if !ok {
		return Result{}, false
	}
	id, ok := m["id"].(float64) // JSON の数値
	if !ok || id != float64(int(id)) {
		return Result{}, false
	}
	status, _ := m["status"].(string)
	switch Status(status) {
	case StatusPicked, StatusCancelled, StatusSaved, StatusFailed:
	default:
		return Result{}, false
	}
	name, _ := m["name"].(string)
	code, _ := m["error"].(string)
	return Result{ID: int(id), Status: Status(status), Name: name, Error: code}, true
}

// BridgeError は Java が {"ok":false,"error":"<code>"} で返した失敗。Code はコードだけで、パス・Uri は含まない。
type BridgeError struct{ Code string }

func (e *BridgeError) Error() string { return "document bridge: " + e.Code }

// Is は Java のコードを、呼び出し側が判断に使うエラー（ErrBusy／ErrNoApp／ErrFolderNotSupported）と対応させる。
func (e *BridgeError) Is(target error) bool {
	switch e.Code {
	case "busy":
		return target == ErrBusy
	case "no_app":
		return target == ErrNoApp
	case "is_directory":
		return target == ErrFolderNotSupported
	}
	return false
}

// ParseEnvelope は Java の {"ok":true,"value":"…"} / {"ok":false,"error":"<code>"} を読む。
func ParseEnvelope(raw string) (value string, err error) {
	var env struct {
		OK    bool   `json:"ok"`
		Value string `json:"value"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return "", fmt.Errorf("document bridge: malformed response: %w", err)
	}
	if !env.OK {
		return "", &BridgeError{Code: env.Error}
	}
	return env.Value, nil
}

// ASCIIJSON は v を JSON にし、ASCII 以外を \uXXXX（UTF-16 のサロゲートペア）に直す。
// JNI の NewStringUTF は Java の "modified UTF-8" を期待し、標準の UTF-8 の 4 バイト（絵文字など）を正しく扱えないため、
// ファイル名を Java へ渡すときは ASCII だけにする。
func ASCIIJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, r := range string(b) {
		if r < 0x80 {
			sb.WriteRune(r)
			continue
		}
		if r1, r2 := utf16.EncodeRune(r); r1 != 0xFFFD || r2 != 0xFFFD {
			fmt.Fprintf(&sb, `\u%04x\u%04x`, r1, r2)
			continue
		}
		fmt.Fprintf(&sb, `\u%04x`, r)
	}
	return sb.String(), nil
}
