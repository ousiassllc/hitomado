// Package platform は Wails v3 の機能（ウィンドウ・トレイ・通知・ダイアログ・イベント）への薄い接続を提供する。
// ロジックは icon／quit／notice に切り出してあり、ここは Wails API を呼ぶだけの層。
package platform

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/ousiassllc/hitomado/internal/capability"
	appevents "github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/exportfile"
	"github.com/ousiassllc/hitomado/internal/notify"
)

// Emitter は events.Emitter の Wails 実装。
type Emitter struct{ app *application.App }

// NewEmitter は Wails アプリのイベントでフロントへ発行する Emitter を返す。
func NewEmitter(app *application.App) *Emitter { return &Emitter{app: app} }

// Emit はイベントをフロントへ発行する。payload が nil ならデータなし。
func (e *Emitter) Emit(name string, payload any) {
	if payload == nil {
		e.app.Event.Emit(name)
		return
	}
	e.app.Event.Emit(name, payload)
}

// FilesDroppedPayload は files-dropped イベントのペイロード（ドロップされたファイルのパス）。
type FilesDroppedPayload struct {
	Files []string `json:"files"`
}

// Files はネイティブの保存ダイアログ（app.FileDialog の実装）。
// Android では保存ダイアログが Storage Access Framework で非同期に結果を返すので、その待ち合わせ（saver）を持つ。
type Files struct {
	app    *application.App
	window func() application.Window
	saver  *exportfile.Saver // Android のみ。ほかの OS は nil
}

// NewFiles は保存ダイアログを main ウィンドウに紐づけて開く FileDialog を返す。
func NewFiles(app *application.App, window func() application.Window) *Files {
	return &Files{app: app, window: window, saver: newSaver(app)}
}

// Browser はシステムブラウザ／メーラーで URL を開く（app.URLOpener の実装）。
type Browser struct{ app *application.App }

// NewBrowser は URLOpener を返す。スキーム検証は呼び出し側が行う。
func NewBrowser(app *application.App) *Browser { return &Browser{app: app} }

// Paths はファイル・フォルダを OS の標準の方法で開く（app.PathOpener の実装）。
type Paths struct{ app *application.App }

// NewPaths は PathOpener を返す。呼び出し側がアプリ自身のデータフォルダのパスだけを渡すこと。
func NewPaths(app *application.App) *Paths { return &Paths{app: app} }

// ShowMainWindow はウィンドウを表示して前面に出す（通知クリック・トレイ・多重起動時）。
func ShowMainWindow(w *application.WebviewWindow) {
	w.Show()
	w.UnMinimise()
	w.Focus()
}

// MinWidth／MinHeight は最小ウィンドウサイズ（NF §7: 3 ペインのため幅 960px）。
const (
	MinWidth  = 960
	MinHeight = 600
)

// MinWindowSize は OS ごとの最小ウィンドウサイズ。タッチ UI の OS（Android）はウィンドウの大きさを OS が決める
// （画面幅に合わせて 1 画面ずつの表示になる。Issue #87）ため、制限しない（0 は制限なし）。
func MinWindowSize(caps capability.Capabilities) (width, height int) {
	if caps.TouchUI {
		return 0, 0
	}
	return MinWidth, MinHeight
}

// NewMainWindow はメインウィンドウを作る。トレイがある OS では、× で終了せずトレイへ格納する（フックで閉じるのを取り消して隠す）。
// ウィンドウ状態を保存する OS（caps.SupportsWindowState）では、saved の位置・サイズで開き、以後の変化を save へ渡す
// （window.state。呼び出し側は戻り値の WindowKeeper の Flush を終了前に呼ぶ）。保存しない OS では saved・save は使わず、WindowKeeper は nil。
func NewMainWindow(app *application.App, caps capability.Capabilities, saved WindowState, save func(WindowState)) (*application.WebviewWindow, *WindowKeeper) {
	if !caps.SupportsWindowState {
		saved = DefaultWindowState()
	}
	minW, minH := MinWindowSize(caps)
	opts := application.WebviewWindowOptions{
		Name:           "main",
		Title:          "Hitomado",
		Width:          saved.Width,
		Height:         saved.Height,
		MinWidth:       minW,
		MinHeight:      minH,
		URL:            "/",
		EnableFileDrop: true,
	}
	if r, ok := saved.Bounds(); ok {
		opts.InitialPosition = application.WindowXY
		opts.X, opts.Y = r.X, r.Y
	}
	w := app.Window.NewWithOptions(opts)
	var keeper *WindowKeeper
	if caps.SupportsWindowState {
		keeper = newWindowKeeper(app, w, saved, save)
	}
	if caps.SupportsTray {
		w.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
			e.Cancel()
			keeper.Flush() // 隠す前に状態を読む
			w.Hide()
		})
	}
	// ファイルのドロップ先は data-file-drop-target 属性の要素のみ。パスは Go でしか得られないため、
	// フロント（AttachmentBar）へ files-dropped として中継する。
	w.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		if files := e.Context().DroppedFiles(); len(files) > 0 {
			app.Event.Emit(appevents.FilesDropped, FilesDroppedPayload{Files: files})
		}
	})
	return w, keeper
}

// Tray はシステムトレイ（開く／終了。未読バッジ付きアイコン）。
type Tray struct {
	tray   *application.SystemTray
	icon   []byte
	badged []byte
	badge  unreadBadge
}

// NewTray はトレイを作る。icon は PNG。クリックと「開く」でウィンドウを表示し、「終了」で quit を呼ぶ。
func NewTray(app *application.App, window *application.WebviewWindow, icon []byte, quit func()) (*Tray, error) {
	badged, err := BadgeIcon(icon)
	if err != nil {
		return nil, err
	}
	t := &Tray{tray: app.SystemTray.New(), icon: icon, badged: badged}
	t.badge.apply = t.applyUnread
	t.tray.SetIcon(icon)
	t.tray.SetTooltip("Hitomado")
	menu := app.Menu.New()
	menu.Add("開く").OnClick(func(*application.Context) { ShowMainWindow(window) })
	menu.AddSeparator()
	menu.Add("終了").OnClick(func(*application.Context) { quit() })
	t.tray.SetMenu(menu)
	t.tray.OnClick(func() { ShowMainWindow(window) })
	return t, nil
}

// SetUnread は未読の有無に応じてトレイアイコンを切り替える。複数のゴルーチンから呼んでよい。
func (t *Tray) SetUnread(has bool) { t.badge.set(has) }

func (t *Tray) applyUnread(has bool) {
	if has {
		t.tray.SetIcon(t.badged)
		t.tray.SetTooltip("Hitomado（未読あり）")
		return
	}
	t.tray.SetIcon(t.icon)
	t.tray.SetTooltip("Hitomado")
}

// Notifier は notify.Notifier の Wails 実装（Windows のトースト通知）。クリックで notify:open-thread を発行する。
type Notifier struct {
	svc    *notifications.NotificationService
	tray   *Tray
	window *application.WebviewWindow
	emit   appevents.Emitter
	log    *slog.Logger
}

// NewNotifier は通知サービスと接続する。svc は application.Options.Services にも登録しておくこと。
func NewNotifier(svc *notifications.NotificationService, tray *Tray, window *application.WebviewWindow, emit appevents.Emitter, log *slog.Logger) *Notifier {
	n := &Notifier{svc: svc, tray: tray, window: window, emit: emit, log: log}
	svc.OnNotificationResponse(n.onResponse)
	return n
}

// NewMail はトーストを出す。本文・件名はログに出さない。
func (n *Notifier) NewMail(_ context.Context, m notify.NewMailNotice) error {
	title, body := noticeText(m)
	if err := n.toast(m.AccountID, m.ThreadID, title, body); err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	return nil
}

// SetUnread はトレイアイコンの未読バッジを切り替える。
func (n *Notifier) SetUnread(has bool) {
	if n.tray != nil {
		n.tray.SetUnread(has)
	}
}

func (n *Notifier) onResponse(r notifications.NotificationResult) {
	if r.Error != nil {
		if !errors.Is(r.Error, context.Canceled) {
			n.log.Warn("notification response failed")
		}
		return
	}
	accountID, _ := r.Response.UserInfo["accountId"].(string)
	threadID, _ := r.Response.UserInfo["threadId"].(string)
	if accountID == "" || threadID == "" {
		return
	}
	ShowMainWindow(n.window)
	n.emit.Emit(appevents.NotifyOpenThread, appevents.OpenThreadPayload{AccountID: accountID, ThreadID: threadID})
}
