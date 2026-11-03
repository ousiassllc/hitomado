// Package asciijson は JNI（Java ↔ Go の文字列の受け渡し）に載せる、ASCII だけの JSON を作る。
package asciijson

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Marshal は v を JSON にし、ASCII 以外を \uXXXX（UTF-16 のサロゲートペア）に直す。
// JNI の NewStringUTF は Java の "modified UTF-8" を期待し、標準の UTF-8 の 4 バイト（絵文字など）を正しく扱えないため、
// Java へ渡す JSON（ファイル名・件名・差出人名など）は ASCII だけにする。Java の JSONObject は \uXXXX を戻す。
func Marshal(v any) (string, error) {
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
