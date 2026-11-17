//go:build windows

package platform

import (
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// appIconResourceID は hitomado.exe に埋め込まれたアイコングループのリソース ID（Wails の既定。build/windows/icon.ico）。
const appIconResourceID = 3

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLoadImage    = user32.NewProc("LoadImageW")
	procSendMessage  = user32.NewProc("SendMessageW")
	procGetDpiForWnd = user32.NewProc("GetDpiForWindow")
	procMetricsForDp = user32.NewProc("GetSystemMetricsForDpi")
	procModuleHandle = kernel32.NewProc("GetModuleHandleW")
)

// KeepWindowIcon は、ウィンドウが表示されるたびに、タスクバー（ICON_BIG）とタイトルバー（ICON_SMALL）のアイコンを
// exe のアイコンリソースから設定し直す。Wails は ICON_BIG しか設定せず、ウィンドウクラスの既定は汎用アイコンのため、
// 上書き更新のあとに Explorer が古いアイコンのキャッシュを使い続けても、起動中のウィンドウは新しいアイコンを自分で持つ（#391）。
func KeepWindowIcon(w *application.WebviewWindow) {
	w.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
		applyWindowIcons(uintptr(w.NativeWindow()), loadAppIcon, setWindowIcon)
	})
}

// loadAppIcon は kind（iconBig／iconSmall）に合う大きさ（hwnd のあるモニターの DPI で決まる）の exe のアイコンを読む。
// LR_SHARED で読むので、返ったハンドルは解放しない。読めなければ 0。
func loadAppIcon(hwnd, kind uintptr) uintptr {
	const (
		smCXIcon, smCYIcon     = 11, 12
		smCXSmIcon, smCYSmIcon = 49, 50
		imageIcon, lrShared    = 1, 0x8000
		defaultDPI             = 96
	)
	// 取得できない（0）ときは、既定の 96 DPI で計算する。
	dpi, _, _ := procGetDpiForWnd.Call(hwnd)
	if dpi == 0 {
		dpi = defaultDPI
	}
	cxIdx, cyIdx := uintptr(smCXIcon), uintptr(smCYIcon)
	if kind == iconSmall {
		cxIdx, cyIdx = smCXSmIcon, smCYSmIcon
	}
	cx, _, _ := procMetricsForDp.Call(cxIdx, dpi)
	cy, _, _ := procMetricsForDp.Call(cyIdx, dpi)
	// 失敗は戻り値 0 で表れる（エラーの詳細は使わない）。呼び出し側が 0 を読み込み失敗として扱う。
	inst, _, _ := procModuleHandle.Call(0)
	icon, _, _ := procLoadImage.Call(inst, appIconResourceID, imageIcon, cx, cy, lrShared)
	return icon
}

// setWindowIcon は WM_SETICON で hwnd のアイコンを差し替える。
func setWindowIcon(hwnd, kind, icon uintptr) {
	const wmSetIcon = 0x0080
	// 戻り値は直前のアイコン。LR_SHARED で読んだものなので解放しない。
	_, _, _ = procSendMessage.Call(hwnd, wmSetIcon, kind, icon)
}
