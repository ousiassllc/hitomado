//go:build windows

package platform

import (
	"syscall"
	"unsafe"
)

// ShowFatal は起動失敗を利用者に知らせるネイティブのメッセージボックスを出す。
// リリースビルドは -H windowsgui で標準エラーが見えず、ウィンドウもないため、これが無いと無反応で終了したように見える。
func ShowFatal(title, message string) {
	t, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	m, err := syscall.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	const mbOK, mbIconError = 0x0, 0x10
	proc := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	_, _, _ = proc.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), uintptr(mbOK|mbIconError)) //nolint:gosec // Win32 API 呼び出しに必要
}
