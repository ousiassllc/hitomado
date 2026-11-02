package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	wevents "github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
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

var mainWindow atomic.Pointer[application.WebviewWindow]

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

// run は composition root。具象をここで組み立て、サービスを Wails に登録して起動する。
func run() error {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// マイグレーションの失敗は起動を中止する（NF §5）。
	st, err := store.Open(ctx, filepath.Join(dir, "hitomado.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = st.Close() }()

	clk := clock.System{}
	sec := secrets.NewKeyring()
	events := &lateEmitter{}
	files, browser, notifier := &lateFiles{}, &lateBrowser{}, &lateNotifier{}
	notifSvc := notifications.New()

	gm, err := gmail.NewClient(gmail.Options{Secrets: sec, Accounts: accountLookup{st}, Clock: clk, Logger: logger})
	if err != nil {
		return fmt.Errorf("gmail client: %w", err)
	}
	sched := syncer.New(syncer.Deps{Store: st, Gmail: gm, Notifier: notifier, Events: events, Clock: clk, Logger: logger})
	flow := oauth.NewFlow(oauth.Options{OpenBrowser: browser.Open})

	var quitter *platform.Quitter
	deps := &app.Deps{
		Store: st, Gmail: gm, Secrets: sec, OAuth: flow, Sync: sched, Notifier: notifier,
		Update: update.NewGitHub(update.Options{UserAgent: "Hitomado/" + version}),
		Events: events, Clock: clk, Files: files, Browser: browser,
		AI: func(provider, model, baseURL, apiKey string) (ai.Provider, error) {
			return ai.New(provider, model, baseURL, apiKey)
		},
		Logger: logger, DataDir: dir, Version: version,
		QuitAck: func() { quitter.Ack() },
	}
	svc := app.NewServices(deps)

	wapp := application.New(application.Options{
		Name:        "Hitomado",
		Description: "複数の Gmail を 1 つの受信箱で扱い、正しい送信元で返信するアプリ",
		Services: []application.Service{
			application.NewService(svc.Account), application.NewService(svc.Inbox),
			application.NewService(svc.Thread), application.NewService(svc.Compose),
			application.NewService(svc.AI), application.NewService(svc.App),
			application.NewService(notifSvc),
		},
		Assets:       application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		MarshalError: apperr.Marshal,
		// 多重起動は禁止し、2 つ目の起動は既存ウィンドウを前面に出して終了する（NF §5）。
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "jp.ousias.hitomado",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if w := mainWindow.Load(); w != nil {
					platform.ShowMainWindow(w)
				}
			},
		},
	})

	win := platform.NewMainWindow(wapp)
	mainWindow.Store(win)
	pe := platform.NewEmitter(wapp)
	events.set(deps.Threads.Emitter(pe))
	files.f.Store(platform.NewFiles(wapp, func() application.Window { return win }))
	browser.b.Store(platform.NewBrowser(wapp))

	quitter = platform.NewQuitter(pe, func() {
		sched.Shutdown()
		wapp.Quit()
	}, nil)
	tray, err := platform.NewTray(wapp, win, appIcon, quitter.Request)
	if err != nil {
		return err
	}
	notifier.n.Store(platform.NewNotifier(notifSvc, tray, win, pe, logger))

	wapp.Event.OnApplicationEvent(wevents.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		startBackground(ctx, deps, svc, logger)
	})
	if err := wapp.Run(); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}

// startBackground は同期ワーカー（needs_reauth 以外）と更新確認ループを起動する。
func startBackground(ctx context.Context, d *app.Deps, svc *app.Services, logger *slog.Logger) {
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
	go svc.App.RunUpdateLoop(ctx)
}
