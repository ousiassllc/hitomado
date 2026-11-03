//go:build !android

package capability

import "runtime"

// Current は実行中の OS の機能（デスクトップ）。
func Current() Capabilities { return Desktop(runtime.GOOS) }
