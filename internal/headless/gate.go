package headless

import (
	"context"
	"sync"
	"time"
)

// appWait は、アプリ本体の起動が実行中のヘッドレス同期の終了を待つ上限。
const appWait = 10 * time.Second

// Gate は、同じプロセスで動く「アプリ本体（Wails。前面の 60 秒ループ）」と「ヘッドレス同期（Worker）」の排他。
//
// Android では WorkManager の Worker はアプリと同じプロセスのバックグラウンドスレッドで動き、Go のランタイムは
// 1 プロセスに 1 つ（libwails.so を読み込んだ時点で始まる）。したがって:
//   - プロセスがアプリ本体を動かしていない（OS がプロセスを止めていた・Worker が起こした）: Worker が自分の store・gmail で 1 回同期し、
//     終わったら閉じる。同期の状態（history_id）と新着の判定は DB が持つため、あとでアプリ本体が起動して同じ DB を開いても、
//     同じ新着を二重に通知しない。
//   - アプリ本体が動いている（Activity が破棄されただけでプロセスと同期ループは残っている場合を含む）: Worker は別の同期を始めず、
//     アプリ本体のスケジューラに「今すぐ 1 回更新」を頼む（60 秒のタイマーが止まっていても、Worker が起きたときに追いつく）。
//     新着の通知はアプリ本体の通知経路が出す。
//   - ヘッドレス同期の実行中にアプリ本体が起動した: ヘッドレス側を取り消して終わりを待ってから、アプリ本体が DB を開く。
type Gate struct {
	mu      sync.Mutex
	app     bool
	refresh func(ctx context.Context) ([]AccountResult, error)
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// AppStarting はアプリ本体の起動（DB を開く前）に呼ぶ。実行中のヘッドレス同期があれば取り消して、終わりを待つ（appWait まで）。
func (g *Gate) AppStarting() {
	g.mu.Lock()
	g.app = true
	var done chan struct{}
	if g.running {
		g.cancel()
		done = g.done
	}
	g.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(appWait):
	}
}

// SetAppRefresh はアプリ本体のスケジューラへ「今すぐ全アカウントを 1 回更新する」窓口を渡す（組み立ての完了後に呼ぶ）。
// それまでの Worker の要求は何もしないで成功にする（アプリ本体が起動中で、起動直後に全アカウントを同期するため）。
func (g *Gate) SetAppRefresh(fn func(ctx context.Context) ([]AccountResult, error)) {
	g.mu.Lock()
	g.refresh = fn
	g.mu.Unlock()
}

// Cancel は実行中のヘッドレス同期を取り消す（Worker が OS に止められたとき）。
func (g *Gate) Cancel() {
	g.mu.Lock()
	if g.cancel != nil {
		g.cancel()
	}
	g.mu.Unlock()
}

// Run は上の規則で fn（ヘッドレス同期）を実行するか、アプリ本体に同期させる。
// ヘッドレス同期が同時に 2 つ走る場合、後の呼び出しは ErrBusy。
func (g *Gate) Run(ctx context.Context, fn func(ctx context.Context) (Result, error)) (Result, error) {
	g.mu.Lock()
	if g.app {
		refresh := g.refresh
		g.mu.Unlock()
		if refresh == nil {
			return Result{AppRunning: true}, nil
		}
		accounts, err := refresh(ctx)
		return Result{AppRunning: true, Accounts: accounts}, err
	}
	if g.running {
		g.mu.Unlock()
		return Result{}, ErrBusy
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	g.running, g.cancel, g.done = true, cancel, done
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		g.running, g.cancel, g.done = false, nil, nil
		g.mu.Unlock()
		cancel()
		close(done)
	}()
	return fn(ctx)
}

// 既定の Gate。run.go（アプリ本体）と JNI の入口（Worker）が共有する。
var gate Gate

// AppStarting は既定の Gate の AppStarting。
func AppStarting() { gate.AppStarting() }

// SetAppRefresh は既定の Gate の SetAppRefresh。
func SetAppRefresh(fn func(ctx context.Context) ([]AccountResult, error)) { gate.SetAppRefresh(fn) }

// Cancel は既定の Gate の Cancel。
func Cancel() { gate.Cancel() }

// AppStopped はアプリ本体が終了した（run が戻った）ことを記録する。以後の Worker は自分で同期する。
func (g *Gate) AppStopped() {
	g.mu.Lock()
	g.app, g.refresh = false, nil
	g.mu.Unlock()
}

// AppStopped は既定の Gate の AppStopped。
func AppStopped() { gate.AppStopped() }
