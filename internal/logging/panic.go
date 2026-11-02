package logging

import (
	"fmt"
	"log/slog"
	"runtime/debug"
)

// LogPanic は捕捉した panic の値とスタックトレースを Error で記録する。where は発生箇所の名前。
// 値は文字列にして Scrub を通る（panic の値に秘密が混ざっても残さない）。
func LogPanic(logger *slog.Logger, where string, recovered any, stack []byte) {
	logger.Error("panic", "where", where, "panic", fmt.Sprint(recovered), "stack", string(stack))
}

// Recover は defer で呼び、panic を記録して握りつぶす（ゴルーチンの最上位用）。
// 握りつぶしてよいのは、その処理が止まってもアプリが動き続けられる場所だけ。
func Recover(logger *slog.Logger, where string) {
	if r := recover(); r != nil {
		LogPanic(logger, where, r, debug.Stack())
	}
}

// RecoverTo は defer で呼び、panic を記録して *errp にエラーとして入れる（run のように呼び出し元へ伝える場所用）。
func RecoverTo(logger *slog.Logger, where string, errp *error) {
	if r := recover(); r != nil {
		LogPanic(logger, where, r, debug.Stack())
		*errp = fmt.Errorf("panic in %s: %v", where, Scrub(fmt.Sprint(r)))
	}
}
