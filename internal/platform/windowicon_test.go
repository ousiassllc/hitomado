package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// setCall は set に渡された引数の記録。
type setCall struct{ hwnd, kind, icon uintptr }

// hwnd があれば、ICON_BIG・ICON_SMALL の両方に、load が返したアイコンを設定する。
func TestApplyWindowIconsSetsBigAndSmall(t *testing.T) {
	t.Parallel()
	var calls []setCall
	load := func(_, kind uintptr) uintptr { return 100 + kind }
	set := func(hwnd, kind, icon uintptr) { calls = append(calls, setCall{hwnd, kind, icon}) }

	applyWindowIcons(42, load, set)

	assert.ElementsMatch(t, []setCall{{42, iconBig, 101}, {42, iconSmall, 100}}, calls)
}

// hwnd が 0（ウィンドウ未作成・破棄済み）なら、読み込みも設定もしない。
func TestApplyWindowIconsSkipsNilWindow(t *testing.T) {
	t.Parallel()
	load := func(_, _ uintptr) uintptr { t.Fatal("load must not be called"); return 0 }
	set := func(_, _, _ uintptr) { t.Fatal("set must not be called") }

	applyWindowIcons(0, load, set)
}

// 読み込めなかった種類（0）は設定しない（既存のアイコンを消さない）。もう一方は設定する。
func TestApplyWindowIconsSkipsFailedLoad(t *testing.T) {
	t.Parallel()
	var calls []setCall
	load := func(_, kind uintptr) uintptr {
		if kind == iconBig {
			return 0
		}
		return 7
	}
	set := func(hwnd, kind, icon uintptr) { calls = append(calls, setCall{hwnd, kind, icon}) }

	applyWindowIcons(1, load, set)

	assert.Equal(t, []setCall{{1, iconSmall, 7}}, calls)
}
