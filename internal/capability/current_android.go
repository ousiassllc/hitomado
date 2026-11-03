//go:build android

package capability

// Current は実行中の OS の機能（Android）。
func Current() Capabilities { return Android() }
