package platform

import (
	"encoding/json"
)

// maxWindowSide はウィンドウの幅・高さとして受け入れる上限（壊れた保存値で巨大なウィンドウを作らない）。
const maxWindowSide = 16384

// DefaultWidth／DefaultHeight は保存された状態がないときの初期サイズ。
const (
	DefaultWidth  = 1280
	DefaultHeight = 800
)

// Rect は DIP 単位の矩形。
type Rect struct{ X, Y, W, H int }

// WindowState は app_settings の window.state に保存するウィンドウの状態（ER §3.9）。
// 位置とサイズは最大化していないときの値（最大化中も、戻したときの大きさを保つ）。X／Y が nil なら位置は未保存。
type WindowState struct {
	X         *int `json:"x,omitempty"`
	Y         *int `json:"y,omitempty"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximized bool `json:"maximized"`
}

// DefaultWindowState は保存値がないときの状態（既定サイズ・位置は OS 任せ）。
func DefaultWindowState() WindowState { return WindowState{Width: DefaultWidth, Height: DefaultHeight} }

// ParseWindowState は保存された JSON を読む。壊れている・寸法が不正なときは ok=false。
// 最小サイズ未満は最小サイズに引き上げる。
func ParseWindowState(raw string) (WindowState, bool) {
	var s WindowState
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return WindowState{}, false
	}
	if s.Width <= 0 || s.Height <= 0 || s.Width > maxWindowSide || s.Height > maxWindowSide {
		return WindowState{}, false
	}
	s.Width = max(s.Width, MinWidth)
	s.Height = max(s.Height, MinHeight)
	if (s.X == nil) != (s.Y == nil) {
		s.X, s.Y = nil, nil
	}
	return s, true
}

// Encode は保存用の JSON にする。
func (s WindowState) Encode() (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err //nolint:wrapcheck // 固定構造体のため実際には失敗しない
	}
	return string(b), nil
}

// Bounds は位置があるときの矩形を返す。
func (s WindowState) Bounds() (Rect, bool) {
	if s.X == nil || s.Y == nil {
		return Rect{}, false
	}
	return Rect{X: *s.X, Y: *s.Y, W: s.Width, H: s.Height}, true
}

// stateOf は最大化していないときの矩形と最大化の有無から保存用の状態を作る。
func stateOf(normal Rect, maximized bool) WindowState {
	x, y := normal.X, normal.Y
	return WindowState{X: &x, Y: &y, Width: normal.W, Height: normal.H, Maximized: maximized}
}

// 画面内とみなすタイトルバー帯の大きさ。ここがいずれかのモニターの作業領域に十分入っていれば、掴んで動かせる。
const (
	grabWidth  = 100
	grabHeight = 24
)

// OnScreen は r のタイトルバー帯（上端 grabHeight の帯）が、いずれかの作業領域と grabWidth×grabHeight 以上重なるかを返す。
// モニターの取り外し・解像度変更で、保存した位置が画面外になっていないかの確認に使う。
func OnScreen(r Rect, workAreas []Rect) bool {
	strip := Rect{X: r.X, Y: r.Y, W: r.W, H: grabHeight}
	for _, a := range workAreas {
		w := min(strip.X+strip.W, a.X+a.W) - max(strip.X, a.X)
		h := min(strip.Y+strip.H, a.Y+a.H) - max(strip.Y, a.Y)
		if w >= grabWidth && h >= grabHeight {
			return true
		}
	}
	return false
}
