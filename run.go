package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	wevents "github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/threadcache"
	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/platform"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/update"
)

// dataDir は %APPDATA%\Hitomado（他 OS では os.UserConfigDir 配下）。
func dataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	dir := filepath.Join(base, "Hitomado")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	return dir, nil
}

// run は composition root。Wails アプリを最初に作る（多重起動の判定は application.New の中で行われ、
// 2 つ目のプロセスは DB・ログ・同期に触れる前に既存ウィンドウを前面に出して終了する）。
// そのあとストアやサービスを組み立て、Run の前にサービスを登録する。
func run() error {
	mainWindow := &windowRef{}
	wapp := application.New(application.Options{
		Name:         "Hitomado",
		Description:  "複数の Gmail を 1 つの受信箱で扱い、正しい送信元で返信するアプリ",
		Assets:       application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		MarshalError: apperr.Marshal,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "jp.ousias.hitomado",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if w := mainWindow.get(); w != nil {
					platform.ShowMainWindow(w)
				}
			},
		},
	})

	dir, err := dataDir()
	if err != nil {
		return err
	}
	logger, closeLog, err := logging.New(filepath.Join(dir, "logs"), slog.LevelInfo, os.Getenv("HITOMADO_LOG_STDERR") != "")
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

	win, keeper := platform.NewMainWindow(wapp, loadWindowState(ctx, st, logger), windowStateSaver(st, logger))
	mainWindow.set(win)
	emitter := platform.NewEmitter(wapp)
	notifSvc := notifications.New()

	var quitter *platform.Quitter
	tray, err := platform.NewTray(wapp, win, appIcon, func() { quitter.Request() })
	if err != nil {
		return err
	}
	notifier := platform.NewNotifier(notifSvc, tray, win, emitter, logger)

	clk := clock.System{}
	sec := secrets.NewKeyring()
	gm, err := gmail.NewClient(gmail.Options{Secrets: sec, Accounts: accountLookup{st}, Clock: clk, Logger: logger})
	if err != nil {
		return fmt.Errorf("gmail client: %w", err)
	}
	cache := threadcache.New(gm, threadcache.DefaultCapacity)
	events := cache.Emitter(emitter)
	sched := syncer.New(syncer.Deps{Store: st, Gmail: gm, Notifier: notifier, Events: events, Clock: clk, Logger: logger})
	flow := oauth.NewFlow(oauth.Options{OpenBrowser: platform.NewBrowser(wapp).Open})

	deps := &app.Deps{
		Store: st, Gmail: gm, Secrets: sec, OAuth: flow, Sync: sched, Notifier: notifier,
		Update: update.NewGitHub(update.Options{UserAgent: "Hitomado/" + version}),
		Events: events, Clock: clk,
		Files:   platform.NewFiles(wapp, func() application.Window { return win }),
		Browser: platform.NewBrowser(wapp),
		AI: func(provider, model, baseURL, apiKey string) (ai.Provider, error) {
			return ai.New(provider, model, baseURL, apiKey)
		},
		Logger: logger, DataDir: dir, Version: version,
		QuitAck: func() { quitter.Ack() },
		Threads: cache,
	}
	svc := app.NewServices(deps)
	for _, s := range []application.Service{
		application.NewService(svc.Account), application.NewService(svc.Inbox),
		application.NewService(svc.Thread), application.NewService(svc.Compose),
		application.NewService(svc.AI), application.NewService(svc.App),
		application.NewService(notifSvc),
	} {
		wapp.RegisterService(s)
	}

	// 終了処理は 1 か所にまとめる: 更新確認ループの停止と完了待ち → 同期の停止 → （呼び出し元の defer で）ストア・ログを閉じる。
	var loops sync.WaitGroup
	shutdown := sync.OnceFunc(func() {
		cancel()
		loops.Wait()
		sched.Shutdown()
	})
	quitter = platform.NewQuitter(emitter, func() {
		keeper.Flush() // 終了でウィンドウが壊れる前に位置・サイズを保存する
		shutdown()
		wapp.Quit()
	}, nil)

	wapp.Event.OnApplicationEvent(wevents.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		startBackground(ctx, deps, &loops, logger)
	})
	err = wapp.Run()
	shutdown()
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}

// startBackground は同期ワーカー（needs_reauth 以外）と更新確認ループを起動する。
func startBackground(ctx context.Context, d *app.Deps, loops *sync.WaitGroup, logger *slog.Logger) {
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
	loops.Add(1)
	go func() {
		defer loops.Done()
		app.NewUpdateLoop(d).Run(ctx)
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
