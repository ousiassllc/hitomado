package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowStateRoundTrip(t *testing.T) {
	in := stateOf(Rect{X: -1200, Y: 40, W: 1100, H: 700}, true)
	raw, err := in.Encode()
	require.NoError(t, err)

	out, ok := ParseWindowState(raw)
	require.True(t, ok)
	r, hasPos := out.Bounds()
	assert.True(t, hasPos)
	assert.Equal(t, Rect{X: -1200, Y: 40, W: 1100, H: 700}, r, "負の座標（左側のモニター）も保つ")
	assert.True(t, out.Maximized)
}

func TestParseWindowState(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		ok      bool
		w, h    int
		hasPos  bool
		maximzd bool
	}{
		{"ER の例（位置なし）", `{"width":1280,"height":800,"maximized":false}`, true, 1280, 800, false, false},
		{"最小未満は最小へ", `{"x":10,"y":10,"width":300,"height":200}`, true, MinWidth, MinHeight, true, false},
		{"片方だけの座標は位置なし扱い", `{"x":10,"width":1000,"height":700}`, true, 1000, 700, false, false},
		{"壊れた JSON", `{`, false, 0, 0, false, false},
		{"寸法 0", `{"width":0,"height":800}`, false, 0, 0, false, false},
		{"巨大な寸法", `{"width":999999,"height":800}`, false, 0, 0, false, false},
		{"空文字", ``, false, 0, 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, ok := ParseWindowState(tt.raw)
			require.Equal(t, tt.ok, ok)
			if !ok {
				return
			}
			assert.Equal(t, tt.w, s.Width)
			assert.Equal(t, tt.h, s.Height)
			_, hasPos := s.Bounds()
			assert.Equal(t, tt.hasPos, hasPos)
			assert.Equal(t, tt.maximzd, s.Maximized)
		})
	}
}

func TestOnScreen(t *testing.T) {
	main := Rect{X: 0, Y: 0, W: 1920, H: 1040}
	left := Rect{X: -1280, Y: 0, W: 1280, H: 1000}
	tests := []struct {
		name  string
		win   Rect
		areas []Rect
		want  bool
	}{
		{"主モニター内", Rect{X: 100, Y: 100, W: 1280, H: 800}, []Rect{main}, true},
		{"左モニター内", Rect{X: -1100, Y: 50, W: 1000, H: 700}, []Rect{main, left}, true},
		{"左モニターが外された", Rect{X: -1100, Y: 50, W: 1000, H: 700}, []Rect{main}, false},
		{"右に大きくはみ出しても掴める部分が残る", Rect{X: 1800, Y: 100, W: 1280, H: 800}, []Rect{main}, true},
		{"掴める部分が 100px 未満", Rect{X: 1850, Y: 100, W: 1280, H: 800}, []Rect{main}, false},
		{"タイトルバーが作業領域より上", Rect{X: 100, Y: -500, W: 1280, H: 800}, []Rect{main}, false},
		{"画面なし", Rect{X: 0, Y: 0, W: 1280, H: 800}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, OnScreen(tt.win, tt.areas))
		})
	}
}
