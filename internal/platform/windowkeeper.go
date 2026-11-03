package platform

import (
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// saveDelay は移動・リサイズの連続イベントをまとめて 1 回だけ保存するための待ち時間。
const saveDelay = 500 * time.Millisecond

// WindowKeeper はメインウィンドウの位置・サイズ・最大化を追跡し、変化から少し後と × で隠すとき、終了前に保存する。
// 起動時は保存値の位置が画面外（モニターの取り外し等）なら中央に戻し、最大化していたなら最大化する。
type WindowKeeper struct {
	app   *application.App
	win   *application.WebviewWindow
	saved WindowState
	save  func(WindowState)

	mu        sync.Mutex
	normal    Rect // 最大化していないときの直近の矩形
	maximized bool
	timer     *time.Timer
}

// newWindowKeeper は win の状態を追跡して save へ渡す。save は保存失敗を自分で記録する。
func newWindowKeeper(app *application.App, win *application.WebviewWindow, saved WindowState, save func(WindowState)) *WindowKeeper {
	k := &WindowKeeper{app: app, win: win, saved: saved, save: save, maximized: saved.Maximized}
	if r, ok := saved.Bounds(); ok {
		k.normal = r
	}
	for _, ev := range []events.WindowEventType{
		events.Common.WindowDidMove, events.Common.WindowDidResize,
		events.Common.WindowMaximise, events.Common.WindowUnMaximise, events.Common.WindowRestore,
	} {
		win.OnWindowEvent(ev, func(*application.WindowEvent) { k.schedule() })
	}
	var once sync.Once
	win.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
		once.Do(func() { go k.restore() })
	})
	return k
}

// restore は最初に表示された後で、画面外の位置を戻し、最大化を復元する。
// 最大化は位置とサイズを設定した後に行う（戻したときの大きさを保存値にするため）。
func (k *WindowKeeper) restore() {
	if r, ok := k.saved.Bounds(); ok {
		var areas []Rect
		for _, s := range k.app.Screen.GetAll() {
			areas = append(areas, Rect{X: s.WorkArea.X, Y: s.WorkArea.Y, W: s.WorkArea.Width, H: s.WorkArea.Height})
		}
		if len(areas) > 0 && !OnScreen(r, areas) {
			k.win.Center()
			k.mu.Lock()
			k.normal = Rect{}
			k.mu.Unlock()
			k.capture()
		}
	}
	if k.saved.Maximized {
		k.win.Maximise()
	}
}

// capture は現在の状態を読み取る。最小化中は何もしない。最大化中は最大化の事実だけを更新する。
func (k *WindowKeeper) capture() {
	if k.win.IsMinimised() {
		return
	}
	maximized := k.win.IsMaximised()
	var b application.Rect
	if !maximized {
		b = k.win.Bounds()
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.maximized = maximized
	if !maximized && b.Width > 0 && b.Height > 0 {
		k.normal = Rect{X: b.X, Y: b.Y, W: b.Width, H: b.Height}
	}
}

// schedule は保存を遅らせて 1 回にまとめる。
func (k *WindowKeeper) schedule() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.timer != nil {
		k.timer.Stop()
	}
	k.timer = time.AfterFunc(saveDelay, k.Flush)
}

// Flush は今の状態を直ちに保存する。ウィンドウが無い（破棄済み）ときは直近に取れた値を保存する。
// k が nil（ウィンドウ状態を保存しない OS）のときは何もしない。
func (k *WindowKeeper) Flush() {
	if k == nil {
		return
	}
	k.capture()
	k.mu.Lock()
	normal, maximized := k.normal, k.maximized
	k.mu.Unlock()
	if normal.W <= 0 || normal.H <= 0 {
		return
	}
	k.save(stateOf(normal, maximized))
}
