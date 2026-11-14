package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	wevents "github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	appdeps "github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/notifycore"
	"github.com/ousiassllc/hitomado/internal/app/threadcache"
	"github.com/ousiassllc/hitomado/internal/app/updatecore"
	"github.com/ousiassllc/hitomado/internal/capability"
	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/e2e"
	appevents "github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/headless"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/platform"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/update"
)

// applyUpdateTimeout は終了時の更新の入れ替え（設定の読み取りとインストーラーの起動）に掛ける時間の上限。
const applyUpdateTimeout = 5 * time.Second

// run は composition root。Wails アプリを最初に作る（多重起動の判定は application.New の中で行われ、
// 2 つ目のプロセスは DB・ログ・同期に触れる前に既存ウィンドウを前面に出して終了する）。
// そのあとストアやサービスを組み立て、Run の前にサービスを登録する。
// MCP サーバー・トレイ・多重起動の防止・ウィンドウ状態の保存復元・アプリ内の自動更新は、OS の機能（capability.Current）が
// 持つときだけ動かす（Android では動かさない。#89）。
func run() error {
	caps := capability.Current()
	mainWindow := &windowRef{}
	wapp := application.New(application.Options{
		Name:         "Hitomado",
		Description:  "複数の Gmail を 1 つの受信箱で扱い、正しい送信元で返信するアプリ",
		Assets:       application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		MarshalError: apperr.Marshal,
		// Wails が捕捉した panic（バインディングの呼び出しなど）を、スタックトレースつきでログに残す。
		// 既定の処理は致命的終了だが、1 つの呼び出しの不具合でアプリごと落とさない（Wails の束縛メソッドと同じ方針）。
		PanicHandler: func(p *application.PanicDetails) {
			logging.LogPanic(slog.Default(), "wails", p.Error, []byte(p.StackTrace))
		},
		SingleInstance: singleInstanceOptions(caps, mainWindow),
	})

	dir, err := dataDir()
	if err != nil {
		return err
	}
	// 同じプロセスで WorkManager の Worker（ヘッドレス同期。Android）が動いていれば取り消して終わりを待ち、DB を同時に開かない。
	// 以後の Worker はこのアプリ本体の同期に任せる（internal/headless の Gate）。
	headless.AppStarting()
	defer headless.AppStopped()
	logger, closeLog, err := logging.New(filepath.Join(dir, app.LogsDirName), slog.LevelInfo, os.Getenv("HITOMADO_LOG_STDERR") != "", logging.Meta{Version: version})
	if err != nil {
		return err
	}
	defer func() { _ = closeLog() }()
	slog.SetDefault(logger)

	// マイグレーションの失敗は起動を中止する（NF §5。main がエラーダイアログを出す）。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st, err := store.Open(ctx, filepath.Join(dir, "hitomado.db"))
	if err != nil {
		return fmt.Errorf("データベースを開けませんでした（%s）: %w", filepath.Join(dir, "hitomado.db"), err)
	}
	defer func() { _ = st.Close() }()
	// AppStopped は st.Close より先に実行する（defer は後入れ先出し）。st を閉じたあとに来た Worker が、閉じた DB に対して
	// SetAppRefresh の更新を呼ばないように、先にアプリ本体の窓口を外して、Worker が自分で同期する側へ倒す。
	// 上の AppStopped は、DB を開く前に戻る経路のため（冪等。両方走っても害はない）。sched は shutdown() で停止済み。
	// run は Wails の Run を伴うためテストでは順序を検査できない。
	defer headless.AppStopped()

	saved, save := platform.DefaultWindowState(), (func(platform.WindowState))(nil)
	if caps.SupportsWindowState {
		saved, save = loadWindowState(ctx, st, logger), windowStateSaver(st, logger)
	}
	win, keeper := platform.NewMainWindow(wapp, caps, saved, save, logger)
	mainWindow.set(win)
	emitter := platform.NewEmitter(wapp)
	notifSvc := notifications.New()

	var quitter *platform.Quitter
	var tray *platform.Tray
	if caps.SupportsTray {
		tray, err = platform.NewTray(wapp, win, func() { quitter.Request() })
		if err != nil {
			return err
		}
	}
	notifier := platform.NewNotifier(notifSvc, tray, win, emitter, logger)

	clk := clock.System{}
	sec := secrets.NewKeyring()
	// 同期・Gmail のログには、ヘッドレス同期（Worker。origin=headless）と区別できるよう origin=app を付ける。
	syncLogger := logging.WithOrigin(logger, logging.OriginApp)
	gm, err := gmail.NewClient(gmail.Options{Secrets: sec, Accounts: headless.StoreLookup{Store: st}, Clock: clk, Logger: syncLogger})
	if err != nil {
		return fmt.Errorf("gmail client: %w", err)
	}
	// E2E 用ビルド（タグ e2e。リリースには入らない）のときだけ、Gmail・資格情報をシード済みのフェイクに差し替える（#90）。
	sec, gm, err = e2e.Override(ctx, dir, st, sec, gm, logger)
	if err != nil {
		return err
	}
	cache := threadcache.New(gm, threadcache.DefaultCapacity)
	events := cache.Emitter(emitter)
	sched := syncer.New(syncer.Deps{Store: st, Gmail: gm, Notifier: notifier, Events: events, Clock: clk, Logger: syncLogger})
	oauthTracker := newOAuthTracker(sec)
	headless.SetAppRefresh(headless.AppRefresh(st, sched, logger))
	flow := oauth.NewFlow(oauth.Options{OpenBrowser: platform.NewBrowser(wapp).Open, Tracker: oauthTracker, Logger: logger})

	// 自動アップデート（NF-D03）は入れ替えを行えるビルド（Windows）の、リリース版（版が 0.0.0 でない）だけ。
	busy := &appdeps.Busy{}
	var updates *update.Manager
	if caps.SupportsAutoUpdate && update.AutoInstallSupported && !update.IsDev(version) {
		updatesDir := filepath.Join(dir, updatecore.DirName)
		updates = update.NewManager(update.ManagerOptions{
			Fetcher: update.NewStager(update.StagerOptions{Dir: updatesDir, UserAgent: "Hitomado/" + version}),
			Launch:  update.NewLauncher(),
			Busy:    busy.Active,
			OnChange: func(st update.State) {
				events.Emit(appevents.UpdateStatus, updatecore.StatusPayload(st))
			},
			Current: version, Dir: updatesDir, Logger: logger,
		})
		defer updates.Close()
	}

	deps := &app.Deps{
		Store: st, Gmail: gm, Secrets: sec, OAuth: flow, OAuthTracker: oauthTracker, Sync: sched, Notifier: notifier,
		Update:  update.NewGitHub(update.Options{UserAgent: "Hitomado/" + version}),
		Updates: updates, Busy: busy,
		Events: events, Clock: clk,
		Files:   platform.NewFiles(wapp, func() application.Window { return win }),
		Browser: platform.NewBrowser(wapp),
		Paths:   platform.NewPaths(wapp),
		AI: func(provider, model, baseURL, apiKey string) (ai.Provider, error) {
			return ai.New(provider, model, baseURL, apiKey)
		},
		AIModels: func(ctx context.Context, provider, baseURL, credential string) ([]ai.ModelInfo, error) {
			return ai.ListModels(ctx, provider, baseURL, credential, nil)
		},
		Classifier: ai.NewClassifier,
		ChatGPT: siwc.New(siwc.Options{
			OpenBrowser: platform.NewBrowser(wapp).Open, Secrets: sec, Settings: st, Clock: clk, Logger: logger,
		}),
		Background:   platform.NewBackground(logger),
		OpenRequests: platform.NewOpenRequests(wapp, emitter),
		Logger:       logger, DataDir: dir, Version: version, Capabilities: caps,
		QuitAck: func() { quitter.Ack() }, RequestQuit: func() { quitter.Request() },
		Threads: cache,
	}
	svc := app.NewServices(deps)
	for _, s := range bindingServices(svc, caps) {
		wapp.RegisterService(s)
	}
	// 通知の許可・OS の通知設定・通知のタップで開くスレッド（全 OS で登録。Windows は unsupported を返す）。
	wapp.RegisterService(application.NewService(notifycore.NewNotifyService((*appdeps.Deps)(deps))))
	// Wails の通知サービスは Android では D-Bus に接続しようとして起動に失敗するため、Android では登録しない（notifsvc_android.go）。
	if s, ok := notificationService(notifSvc); ok {
		wapp.RegisterService(s)
	}

	// 終了処理は 1 か所にまとめる: 更新確認ループの停止と完了待ち → 同期の停止 → （呼び出し元の defer で）ストア・ログを閉じる。
	var loops sync.WaitGroup
	shutdown := sync.OnceFunc(func() {
		// Run がトレイ終了を経由せず戻った場合、flush 中の tray.SetIcon（InvokeSync）で sched.Shutdown が戻らないことがある。
		// 終了処理全体（ループ・MCP・同期の停止）に上限を付け、超えたら打ち切って先へ進む。
		stopped := platform.WaitBounded(logger, func() {
			cancel()
			loops.Wait()
			svc.MCP.Shutdown()
			sched.Shutdown()
		}, platform.ShutdownGrace)
		if !stopped {
			logger.Warn("shutdown timed out; continuing", "grace", platform.ShutdownGrace.String())
		}
	})
	// 更新の入れ替え（NF-D03）は、同期・ループを止めたあと、終了の直前に 1 回だけ行う（インストーラーは別プロセスで起動し、
	// アプリの終了を待ってから上書きする）。ストア・ログは呼び出し元の defer で閉じるので、設定はここで読める。
	applyUpdate := sync.OnceFunc(func() {
		ctx, cancel := context.WithTimeout(context.Background(), applyUpdateTimeout)
		defer cancel()
		updatecore.New((*appdeps.Deps)(deps)).ApplyOnExit(ctx)
	})
	quitter = platform.NewQuitter(logger, emitter, func() {
		keeper.Flush() // 終了でウィンドウが壊れる前に位置・サイズを保存する
		shutdown()
		applyUpdate()
		wapp.Quit()
	}, nil)

	wapp.Event.OnApplicationEvent(wevents.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		startBackground(ctx, deps, svc, &loops, logger)
	})
	err = wapp.Run()
	shutdown()
	applyUpdate()
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}

// startBackground は MCP サーバー（OS が持ち、設定でオンのときだけ）・同期ワーカー（needs_reauth 以外）・
// 更新確認ループ（OS が持つときだけ）を起動する。
func startBackground(ctx context.Context, d *app.Deps, svc *app.Services, loops *sync.WaitGroup, logger *slog.Logger) {
	svc.MCP.Start(ctx) // OS が持ち、設定でオンのときだけ待ち受ける（既定はオフ。Android では保存値がオンでも待ち受けない）
	accounts, err := d.Store.ListAccounts(ctx)
	if err != nil {
		logger.Error("list accounts failed", "err", err.Error())
		return
	}
	for _, a := range accounts {
		if a.Status != store.StatusNeedsReauth {
			d.Sync.Start(a.ID)
		}
	}
	// バックグラウンド同期（Android の WorkManager）。アカウントがあれば登録（冪等）、無ければ解除する。Windows では何もしない。
	// E2E 用ビルドではフェイクの資格情報がアプリ内にしか無く、Worker（ヘッドレス）は同期できないので登録しない。
	if !e2e.Enabled {
		d.Background.ScheduleSync(len(accounts) > 0)
	}
	if !d.Capabilities.SupportsAutoUpdate {
		return
	}
	loops.Add(1)
	go func() {
		defer loops.Done()
		defer logging.Recover(logger, "update loop")
		updatecore.NewLoop((*appdeps.Deps)(d)).Run(ctx)
	}()
}

// windowRef は多重起動コールバックからメインウィンドウを参照するための入れ物（ウィンドウは application.New の後に作る）。
type windowRef struct {
	mu sync.Mutex
	w  *application.WebviewWindow
}

func (r *windowRef) set(w *application.WebviewWindow) { r.mu.Lock(); r.w = w; r.mu.Unlock() }

func (r *windowRef) get() *application.WebviewWindow {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.w
}
