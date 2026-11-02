package syncer

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/notify"
	"github.com/ousiassllc/hitomado/internal/store"
)

const (
	defaultInterval  = 60 * time.Second
	refreshSuppress  = 5 * time.Second // 手動更新の抑制時間（NF §4）
	retentionEvery   = 24 * time.Hour  // 保持期間削除の実行間隔
	retentionPeriod  = 90 * 24 * time.Hour
	initialPageSize  = 200 // 初回同期・リセット同期で取得する INBOX のスレッド数
	morePageSize     = 100 // FetchMore の 1 回あたりの取得数
	fetchConcurrency = 4   // メタデータ取得の同時数
)

// ErrClosed は Shutdown 済みのスケジューラへの要求に対するエラー。
var ErrClosed = errors.New("syncer: scheduler is shut down")

// Deps は Scheduler の依存。Store・Gmail は必須。
type Deps struct {
	Store    store.Store
	Gmail    gmail.Client
	Notifier notify.Notifier // nil は通知なし
	Events   events.Emitter  // nil はイベントなし
	Clock    clock.Clock     // nil は実時間
	Interval time.Duration   // 0 は 60 秒
	// NewTicker は周期通知チャネルと停止関数を返す。nil は time.NewTicker。テストで手動駆動するために差し替える。
	NewTicker func(d time.Duration) (<-chan time.Time, func())
	Logger    *slog.Logger // nil は破棄
}

// Scheduler は Engine の実装。アカウントごとに 1 ワーカー（ゴルーチン）で差分同期を回す。
type Scheduler struct {
	store     store.Store
	gmail     gmail.Client
	notifier  notify.Notifier
	events    events.Emitter
	clock     clock.Clock
	interval  time.Duration
	newTicker func(d time.Duration) (<-chan time.Time, func())
	log       *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup // ワーカー・手動更新・保持期間削除のゴルーチン

	mu        sync.Mutex // 以下を保護
	closed    bool
	accounts  map[string]*account
	refreshes map[string]*refreshCall
}

var _ Engine = (*Scheduler)(nil)

// account はアカウント 1 件分の実行時状態。
type account struct {
	id     string
	ctx    context.Context // Stop／Shutdown でキャンセルされる
	cancel context.CancelFunc

	syncMu sync.Mutex  // 同期周期・FetchMore の直列化（同期状態の読み書きを守る）
	paused atomic.Bool // needs_reauth のため自動再試行しない

	started bool // s.mu で保護
	done    chan struct{}
	kick    chan struct{}
}

// New は Scheduler を作り、保持期間削除のゴルーチンを起動する。
func New(d Deps) *Scheduler {
	s := &Scheduler{
		store:     d.Store,
		gmail:     d.Gmail,
		notifier:  d.Notifier,
		events:    d.Events,
		clock:     d.Clock,
		interval:  d.Interval,
		newTicker: d.NewTicker,
		log:       d.Logger,
		accounts:  map[string]*account{},
		refreshes: map[string]*refreshCall{},
	}
	if s.notifier == nil {
		s.notifier = noopNotifier{}
	}
	if s.events == nil {
		s.events = noopEmitter{}
	}
	if s.clock == nil {
		s.clock = clock.System{}
	}
	if s.interval <= 0 {
		s.interval = defaultInterval
	}
	if s.newTicker == nil {
		s.newTicker = func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		}
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.wg.Add(1)
	go s.runRetention()
	return s
}

// accountLocked は id の account を返す（無ければ作る）。s.mu を保持して呼ぶ。
func (s *Scheduler) accountLocked(id string) *account {
	a, ok := s.accounts[id]
	if !ok {
		a = &account{id: id, done: make(chan struct{}), kick: make(chan struct{}, 1)}
		a.ctx, a.cancel = context.WithCancel(s.ctx)
		s.accounts[id] = a
	}
	return a
}

// Start はワーカーを起動する（冪等）。起動直後に 1 周期同期する。
// needs_reauth で停止中のワーカーは再開する（再接続時）。
func (s *Scheduler) Start(accountID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	a := s.accountLocked(accountID)
	delete(s.refreshes, accountID)
	if a.started {
		if a.paused.CompareAndSwap(true, false) {
			a.signal()
		}
		return
	}
	a.started = true
	a.paused.Store(false)
	s.wg.Add(1)
	go s.runWorker(a)
}

// Stop はワーカーを止め、実行中の同期周期の終了を待つ（アカウント削除の前に呼ぶ）。
func (s *Scheduler) Stop(accountID string) {
	s.mu.Lock()
	a, ok := s.accounts[accountID]
	delete(s.accounts, accountID)
	delete(s.refreshes, accountID)
	s.mu.Unlock()
	if !ok {
		return
	}
	a.cancel()
	if a.started {
		<-a.done
	}
	a.waitIdle() // 実行中の手動更新・FetchMore の完了待ち
}

// Trigger は非同期に差分同期を要求する。空は起動中の全ワーカー。
func (s *Scheduler) Trigger(accountIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(accountIDs) == 0 {
		for _, a := range s.accounts {
			if a.started {
				a.signal()
			}
		}
		return
	}
	for _, id := range accountIDs {
		if a, ok := s.accounts[id]; ok && a.started {
			a.signal()
		}
	}
}

// Shutdown は全ワーカーを止め、ゴルーチンの終了を待つ。複数回呼んでよい。
func (s *Scheduler) Shutdown() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	s.wg.Wait()
}

func (a *account) signal() {
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

// runWorker はアカウントの周期同期ループ。needs_reauth の間は周期・要求を無視する。
func (s *Scheduler) runWorker(a *account) {
	defer s.wg.Done()
	defer close(a.done)
	tick, stop := s.newTicker(s.interval)
	defer stop()
	s.cycle(a)
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-tick:
			s.cycle(a)
		case <-a.kick:
			s.cycle(a)
		}
	}
}

func (s *Scheduler) cycle(a *account) {
	defer logging.Recover(s.log, "sync cycle") // 1 周期の panic でワーカーを止めない（スタックを記録して次の周期へ）
	if a.paused.Load() {
		return
	}
	if err := s.runCycle(a.ctx, a); err != nil && a.ctx.Err() == nil {
		s.log.Warn("sync cycle failed", "account", a.id, "err", err)
	}
}

// runCycle は 1 周期の同期を直列化して実行する。
func (s *Scheduler) runCycle(ctx context.Context, a *account) error {
	a.syncMu.Lock()
	defer a.syncMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.paused.Load() {
		return errNeedsReauth(a.id)
	}
	return s.syncAccount(ctx, a)
}

// runRetention は保持期間を過ぎたスレッドを起動直後と 24 時間ごとに削除する。
func (s *Scheduler) runRetention() {
	defer s.wg.Done()
	tick, stop := s.newTicker(retentionEvery)
	defer stop()
	s.purge()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-tick:
			s.purge()
		}
	}
}

func (s *Scheduler) purge() {
	n, err := s.store.PurgeThreadsLeftInboxBefore(s.ctx, s.clock.Now().Add(-retentionPeriod))
	if err != nil {
		if s.ctx.Err() == nil {
			s.log.Warn("retention purge failed", "err", err)
		}
		return
	}
	if n > 0 {
		s.log.Info("retention purge", "deleted", n)
	}
}

// joinCtx は ctx が a の終了でもキャンセルされるようにする。
func joinCtx(ctx context.Context, a *account) (context.Context, context.CancelFunc) {
	c, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(a.ctx, cancel)
	return c, func() {
		stop()
		cancel()
	}
}

type noopNotifier struct{}

func (noopNotifier) NewMail(context.Context, notify.NewMailNotice) error { return nil }
func (noopNotifier) SetUnread(bool)                                      {}

type noopEmitter struct{}

func (noopEmitter) Emit(string, any) {}

// waitIdle は実行中の同期周期・FetchMore の終了を待つ。
func (a *account) waitIdle() {
	a.syncMu.Lock()
	defer a.syncMu.Unlock()
}
