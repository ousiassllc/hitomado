package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMascotPosition(t *testing.T) {
	cases := []struct {
		name         string
		screens      []screenArea
		wantX, wantY int
		wantOK       bool
	}{
		{
			name:    "プライマリの作業領域の右下から余白を空ける（タスクバー分は作業領域に含まれない）",
			screens: []screenArea{{work: Rect{X: 0, Y: 0, W: 1920, H: 1040}, primary: true}},
			wantX:   1920 - MascotSize - MascotMargin, wantY: 1040 - MascotSize - MascotMargin, wantOK: true,
		},
		{
			name: "複数モニターではプライマリを選ぶ（先頭でなくても）",
			screens: []screenArea{
				{work: Rect{X: 1920, Y: 0, W: 1280, H: 1024}},
				{work: Rect{X: 0, Y: 0, W: 1920, H: 1040}, primary: true},
			},
			wantX: 1920 - MascotSize - MascotMargin, wantY: 1040 - MascotSize - MascotMargin, wantOK: true,
		},
		{
			name: "プライマリが左上のモニターの右下にあり、作業領域の原点がマイナスでも、作業領域の右下に置く",
			screens: []screenArea{
				{work: Rect{X: -1920, Y: -200, W: 1920, H: 1080}},
				{work: Rect{X: 0, Y: 0, W: 2560, H: 1400}, primary: true},
			},
			wantX: 2560 - MascotSize - MascotMargin, wantY: 1400 - MascotSize - MascotMargin, wantOK: true,
		},
		{
			name:    "プライマリ自身の作業領域が負の座標でも、原点ではなく作業領域の X・Y から数える",
			screens: []screenArea{{work: Rect{X: -1920, Y: -1080, W: 1920, H: 1040}, primary: true}},
			wantX:   -MascotSize - MascotMargin, wantY: -1080 + 1040 - MascotSize - MascotMargin, wantOK: true,
		},
		{
			name:    "プライマリの印が無ければ先頭のモニター",
			screens: []screenArea{{work: Rect{X: 100, Y: 50, W: 800, H: 600}}, {work: Rect{X: 900, Y: 0, W: 800, H: 600}}},
			wantX:   100 + 800 - MascotSize - MascotMargin, wantY: 50 + 600 - MascotSize - MascotMargin, wantOK: true,
		},
		{
			name:    "作業領域が小さすぎても、左上は作業領域の外へ出さない",
			screens: []screenArea{{work: Rect{X: -300, Y: 40, W: 60, H: 70}, primary: true}},
			wantX:   -300, wantY: 40, wantOK: true,
		},
		{name: "モニターが無ければ置けない", screens: nil, wantOK: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x, y, ok := mascotPosition(c.screens)
			assert.Equal(t, c.wantOK, ok)
			assert.Equal(t, c.wantX, x)
			assert.Equal(t, c.wantY, y)
		})
	}
}
