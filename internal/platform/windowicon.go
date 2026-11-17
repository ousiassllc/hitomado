package platform

// WM_SETICON の wParam。ICON_SMALL はタイトルバー・Alt+Tab の小さい方、ICON_BIG はタスクバー・Alt+Tab の大きい方に使われる。
const (
	iconSmall uintptr = 0
	iconBig   uintptr = 1
)

// applyWindowIcons は hwnd に、アプリのアイコン（load が kind ごとに返すハンドル）を ICON_BIG・ICON_SMALL の両方で設定する。
// hwnd が 0（ウィンドウ未作成・破棄済み）なら何もしない。読み込めなかった（0）種類は設定しない（既存のアイコンを消さない）。
// 実際の Win32 呼び出しは windowicon_windows.go が load・set として渡す。
func applyWindowIcons(hwnd uintptr, load func(hwnd, kind uintptr) uintptr, set func(hwnd, kind, icon uintptr)) {
	if hwnd == 0 {
		return
	}
	for _, kind := range []uintptr{iconBig, iconSmall} {
		if icon := load(hwnd, kind); icon != 0 {
			set(hwnd, kind, icon)
		}
	}
}
