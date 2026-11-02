package apptest

import (
	"context"
	"sync"

	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/notify"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/update"
)

// FakeFlow は oauth.Flow のフェイク。Result／Err を返す。Block=true のときは ctx キャンセルか Complete まで待つ
// （キャンセルは oauth.ErrCancelled）。Authorize に入るたびに Started へ通知する（待ちの同期に使う）。
type FakeFlow struct {
	mu sync.Mutex

	Result    oauth.Result
	Err       error
	Block     bool
	RevokeErr error
	Started   chan struct{} // バッファ 16。満杯なら通知を捨てる

	Configs []oauth.ClientConfig
	Revoked []string

	release chan flowOutcome
}

type flowOutcome struct {
	res oauth.Result
	err error
}

var _ oauth.Flow = (*FakeFlow)(nil)

// NewFakeFlow は FakeFlow を返す。
func NewFakeFlow() *FakeFlow {
	return &FakeFlow{Started: make(chan struct{}, 16), release: make(chan flowOutcome, 1)}
}

// Complete は Block 中の Authorize を指定の結果で終わらせる。
func (f *FakeFlow) Complete(res oauth.Result, err error) { f.release <- flowOutcome{res, err} }

// Authorize は設定どおりの結果を返す。
func (f *FakeFlow) Authorize(ctx context.Context, cfg oauth.ClientConfig) (oauth.Result, error) {
	f.mu.Lock()
	f.Configs = append(f.Configs, cfg)
	block, res, err := f.Block, f.Result, f.Err
	f.mu.Unlock()
	select {
	case f.Started <- struct{}{}:
	default:
	}
	if !block {
		return res, err
	}
	select {
	case <-ctx.Done():
		return oauth.Result{}, oauth.ErrCancelled
	case o := <-f.release:
		return o.res, o.err
	}
}

// Revoke は Revoked に記録し RevokeErr を返す。
func (f *FakeFlow) Revoke(_ context.Context, refreshToken string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Revoked = append(f.Revoked, refreshToken)
	return f.RevokeErr
}

// FakeFiles は app.FileDialog のフェイク。Cancel=true で「キャンセル」、それ以外は Path を返す。
type FakeFiles struct {
	mu sync.Mutex

	Path      string
	Cancel    bool
	Err       error
	Suggested []string // SaveFile に渡された suggestedName
}

// SaveFile は設定どおりの結果を返す。
func (f *FakeFiles) SaveFile(_ context.Context, suggestedName string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Suggested = append(f.Suggested, suggestedName)
	if f.Err != nil {
		return "", false, f.Err
	}
	if f.Cancel {
		return "", false, nil
	}
	return f.Path, true, nil
}

// FakeBrowser は app.URLOpener のフェイク。開いた URL を記録する。
type FakeBrowser struct {
	mu sync.Mutex

	URLs []string
	Err  error
}

// Open は URL を記録する。
func (f *FakeBrowser) Open(url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.URLs = append(f.URLs, url)
	return f.Err
}

// FakeNotifier は notify.Notifier のフェイク。
type FakeNotifier struct {
	mu sync.Mutex

	Notices []notify.NewMailNotice
	Unread  []bool
}

var _ notify.Notifier = (*FakeNotifier)(nil)

// NewMail は通知を記録する。
func (f *FakeNotifier) NewMail(_ context.Context, n notify.NewMailNotice) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Notices = append(f.Notices, n)
	return nil
}

// SetUnread は呼び出しを記録する。
func (f *FakeNotifier) SetUnread(hasUnread bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Unread = append(f.Unread, hasUnread)
}

// Emitted は FakeEmitter が記録した 1 件のイベント。
type Emitted struct {
	Name    string
	Payload any
}

// FakeEmitter は events.Emitter のフェイク。発行を Events に記録する。並行利用できる。
// 発行を待つ場合は Wait を使う（固定時間の待ちに頼らない）。
type FakeEmitter struct {
	mu   sync.Mutex
	cond *sync.Cond

	Events []Emitted
}

var _ events.Emitter = (*FakeEmitter)(nil)

// NewFakeEmitter は FakeEmitter を返す。
func NewFakeEmitter() *FakeEmitter {
	f := &FakeEmitter{}
	f.cond = sync.NewCond(&f.mu)
	return f
}

// Emit はイベントを記録する。
func (f *FakeEmitter) Emit(name string, payload any) {
	f.mu.Lock()
	f.Events = append(f.Events, Emitted{Name: name, Payload: payload})
	f.mu.Unlock()
	f.cond.Broadcast()
}

// Named は name のイベントを発行順に返す。
func (f *FakeEmitter) Named(name string) []Emitted {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Emitted
	for _, e := range f.Events {
		if e.Name == name {
			out = append(out, e)
		}
	}
	return out
}

// Wait は name のイベントが n 件以上になるまで待ち、その時点の一覧を返す。ctx キャンセルで nil を返す。
func (f *FakeEmitter) Wait(ctx context.Context, name string, n int) []Emitted {
	stop := context.AfterFunc(ctx, func() {
		f.mu.Lock()
		f.cond.Broadcast() // 待機側が cond.Wait に入る前の取りこぼしを防ぐため、ロック下で起こす
		f.mu.Unlock()
	})
	defer stop()
	f.mu.Lock()
	defer f.mu.Unlock()
	for {
		var out []Emitted
		for _, e := range f.Events {
			if e.Name == name {
				out = append(out, e)
			}
		}
		if len(out) >= n {
			return out
		}
		if ctx.Err() != nil {
			return nil
		}
		f.cond.Wait()
	}
}

// FakeUpdate は update.Checker のフェイク。
type FakeUpdate struct {
	mu sync.Mutex

	Release update.Release
	Err     error
	Calls   int
}

var _ update.Checker = (*FakeUpdate)(nil)

// Latest は設定どおりの結果を返す。
func (f *FakeUpdate) Latest(context.Context) (update.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	return f.Release, f.Err
}

// Set は実行中の Latest と競合しないように結果を差し替える。
func (f *FakeUpdate) Set(rel update.Release, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Release, f.Err = rel, err
}

// CallCount は Latest の呼び出し回数を返す。
func (f *FakeUpdate) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Calls
}

// FailingSecrets は secrets.Store を包み、SetErr／DeleteErr が非 nil の間は Set／Delete を（書き込まずに）失敗させる。
// Deps.Secrets へ差し替えて使う。
type FailingSecrets struct {
	secrets.Store

	mu        sync.Mutex
	setErr    error
	deleteErr error
}

// FailSet は以降の Set を err で失敗させる（nil で解除）。
func (f *FailingSecrets) FailSet(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setErr = err
}

// FailDelete は以降の Delete を err で失敗させる（nil で解除）。
func (f *FailingSecrets) FailDelete(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteErr = err
}

// Set は secrets.Store.Set。FailSet 中は失敗する。
func (f *FailingSecrets) Set(key, value string) error {
	f.mu.Lock()
	err := f.setErr
	f.mu.Unlock()
	if err != nil {
		return err
	}
	return f.Store.Set(key, value)
}

// Delete は secrets.Store.Delete。FailDelete 中は失敗する。
func (f *FailingSecrets) Delete(key string) error {
	f.mu.Lock()
	err := f.deleteErr
	f.mu.Unlock()
	if err != nil {
		return err
	}
	return f.Store.Delete(key)
}

var _ secrets.Store = (*FailingSecrets)(nil)
