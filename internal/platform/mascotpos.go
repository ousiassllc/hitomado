package platform

// マスコットのウィンドウの大きさと、画面の端からの余白（DIP）。
const (
	MascotSize   = 72
	MascotMargin = 16
)

// screenArea は 1 つのモニターの作業領域（タスクバーを除く。DIP）と、プライマリかどうか。
type screenArea struct {
	work    Rect
	primary bool
}

// mascotPosition はマスコットの左上の座標を返す。プライマリのモニター（無ければ先頭）の作業領域の右下に、
// 余白 MascotMargin を空けて置く。作業領域の座標は負にもなる（プライマリの左・上にモニターがあるとき）ので、
// 原点を 0 とみなさず作業領域の X・Y から数える。作業領域が小さすぎるときも、左上は作業領域の外へ出さない。
// モニターが 1 つも無いときは ok=false。
func mascotPosition(screens []screenArea) (x, y int, ok bool) {
	if len(screens) == 0 {
		return 0, 0, false
	}
	area := screens[0].work
	for _, s := range screens {
		if s.primary {
			area = s.work
			break
		}
	}
	x = max(area.X+area.W-MascotSize-MascotMargin, area.X)
	y = max(area.Y+area.H-MascotSize-MascotMargin, area.Y)
	return x, y, true
}
