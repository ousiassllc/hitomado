package platform

import (
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/ousiassllc/hitomado/internal/capability"
	"github.com/ousiassllc/hitomado/internal/logging"
)

// Mascot は画面の右下の端に残るエージェントのマスコット（deps.MascotWindow の実装。Windows のみ。#372）。
// メインとは別の WebviewWindow で、枠なし・常に最前面・タスクバーに出さない・背景透過。中身は同じアセットの /#mascot。
//
// メインウィンドウの × は WindowClosing を取り消して隠すだけ（NewMainWindow）でアプリは終了しないので、マスコットは残る。
// マスコット自身の × 相当（Alt+F4 など）も終了ではなく隠すだけで、設定（マスコットのオン）は変えない。
// アプリの終了（トレイの「終了」）では、すべてのウィンドウと一緒に消える。
type Mascot struct {
	app  *application.App
	caps capability.Capabilities
	log  *slog.Logger

	mu  sync.Mutex
	win *application.WebviewWindow // 初めて出すときに作り、以後は作り直さず隠す・出すだけ
}

// NewMascot はマスコットを返す（この時点ではウィンドウを作らない）。マスコットの押下は OpenAgentPanel（Go 側が ShowMainWindow を呼ぶ）。
func NewMascot(app *application.App, caps capability.Capabilities, log *slog.Logger) *Mascot {
	return &Mascot{app: app, caps: caps, log: log}
}

// SetVisible はマスコットを出す・消す。冪等で、複数のゴルーチンから呼んでよい。マスコットに対応しない OS では何もしない。
// panic はここで記録して握りつぶす（呼び出し元のゴルーチン・アプリごと落とさない）。
func (m *Mascot) SetVisible(visible bool) {
	if m == nil || !m.caps.SupportsMascot {
		return
	}
	defer logging.Recover(m.log, "mascot set visible")
	m.mu.Lock()
	defer m.mu.Unlock()
	if !visible {
		if m.win != nil {
			m.win.Hide()
		}
		return
	}
	if m.win == nil {
		m.win = m.create()
		return // 作成と同時に表示される
	}
	m.win.Show()
}

// create はマスコットのウィンドウを作る（表示された状態で）。
func (m *Mascot) create() *application.WebviewWindow {
	opts := application.WebviewWindowOptions{
		Name:   "mascot",
		Title:  "Hitomado エージェント",
		Width:  MascotSize,
		Height: MascotSize,
		// Windows は枠のあるウィンドウの最小幅を OS が決める（既定で 130 前後）ので、最小・最大を大きさに揃えて 72px を保つ。
		MinWidth:    MascotSize,
		MinHeight:   MascotSize,
		MaxWidth:    MascotSize,
		MaxHeight:   MascotSize,
		URL:         "/#mascot",
		Frameless:   true,
		AlwaysOnTop: true,
		// 透過。背景色は無視される（フロントも html・body を透明にする）。
		BackgroundType:             application.BackgroundTypeTransparent,
		DisableResize:              true,
		DefaultContextMenuDisabled: true,
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true, // WS_EX_TOOLWINDOW。タスクバーにも Alt+Tab にも出さない
			// 枠なしのウィンドウに付く影・角丸の枠を描かない（透過した丸の外に四角い影が出ないように）。
			DisableFramelessWindowDecorations: true,
		},
	}
	if x, y, ok := mascotPosition(m.screenAreas()); ok {
		opts.InitialPosition = application.WindowXY
		opts.X, opts.Y = x, y
	}
	w := m.app.Window.NewWithOptions(opts)
	// Windows は生成時に最小幅が効かず OS の既定の最小幅になりうるので、生成直後に大きさを確定させる。
	w.SetSize(MascotSize, MascotSize)
	// 画面の構成が変わっても、出すたびに右下へ置き直す（起動の早い時期に画面の一覧が空だったときも、ここで直る）。
	w.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
		go m.place(w)
	})
	// × 相当（Alt+F4 など）は終了ではなく隠すだけ。設定はオンのまま（次に出すときは設定の切り替えか再起動）。
	w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		w.Hide()
	})
	return w
}

// place はマスコットをプライマリの作業領域の右下へ動かす。別のゴルーチンから呼ぶ。
func (m *Mascot) place(w *application.WebviewWindow) {
	defer logging.Recover(m.log, "mascot place")
	if x, y, ok := mascotPosition(m.screenAreas()); ok {
		w.SetPosition(x, y)
	}
}

// screenAreas は Wails の画面の一覧を作業領域（DIP）の一覧にする。
func (m *Mascot) screenAreas() []screenArea {
	screens := m.app.Screen.GetAll()
	areas := make([]screenArea, 0, len(screens))
	for _, s := range screens {
		areas = append(areas, screenArea{
			work:    Rect{X: s.WorkArea.X, Y: s.WorkArea.Y, W: s.WorkArea.Width, H: s.WorkArea.Height},
			primary: s.IsPrimary,
		})
	}
	return areas
}
