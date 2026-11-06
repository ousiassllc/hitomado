package logging

import (
	"fmt"
	"strings"
)

// maxChainDepth は ErrorChain がたどるラップの段数の上限（循環した Unwrap でも止まるように）。
const maxChainDepth = 16

// ErrorChain は err の Unwrap のつながりを、型名だけで "*fmt.wrapError > *url.Error > *net.OpError" の形にして返す。
// ラップされた元のエラーが何だったか（Wails やネットワーク層のどこで起きたか）を、メッセージの文面を載せずに追うためのもの。
// 値（メッセージ）は含まないので、秘密を運ばない。errors.Join の複数の枝は "[a|b]" に畳む。nil は空文字列。
func ErrorChain(err error) string {
	return errorChain(err, maxChainDepth)
}

func errorChain(err error, depth int) string {
	var parts []string
	for ; err != nil && depth > 0; depth-- {
		parts = append(parts, fmt.Sprintf("%T", err))
		switch u := err.(type) { //nolint:errorlint // 連鎖を 1 段ずつたどるので、As ではなく各段の型そのものを見る
		case interface{ Unwrap() error }:
			err = u.Unwrap()
		case interface{ Unwrap() []error }:
			branches := make([]string, 0, 2)
			for _, e := range u.Unwrap() {
				if s := errorChain(e, depth-1); s != "" {
					branches = append(branches, s)
				}
			}
			if len(branches) > 0 {
				parts = append(parts, "["+strings.Join(branches, "|")+"]")
			}
			return strings.Join(parts, " > ")
		default:
			err = nil
		}
	}
	return strings.Join(parts, " > ")
}
