package syncertests

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

// skipLog は「paused のため同期周期を見送った」ログをアカウントごとに数える slog.Handler。
// ワーカーが tick を処理し終えたことの観測に使う。
type skipLog struct {
	mu     sync.Mutex
	count  map[string]int
	panics int // "panic" レコード（logging.Recover が書く）の数
	// dropped は新着の観測口の「候補が上限を超えて捨てられた」ログの count の合計。
	dropped int64
}

const skippedMsg = "sync cycle skipped: paused"

func (l *skipLog) Enabled(context.Context, slog.Level) bool { return true }
func (l *skipLog) WithAttrs([]slog.Attr) slog.Handler       { return l }
func (l *skipLog) WithGroup(string) slog.Handler            { return l }
func (l *skipLog) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "panic" {
		l.mu.Lock()
		l.panics++
		l.mu.Unlock()
		return nil
	}
	if r.Message == "new mail candidates dropped" {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "count" {
				l.mu.Lock()
				l.dropped += a.Value.Int64()
				l.mu.Unlock()
			}
			return true
		})
		return nil
	}
	if r.Message != skippedMsg {
		return nil
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "account" {
			l.mu.Lock()
			l.count[a.Value.String()]++
			l.mu.Unlock()
			return false
		}
		return true
	})
	return nil
}

func (l *skipLog) panicCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.panics
}

func (l *skipLog) skipped(id string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.count[id]
}

const waitTimeout = 5 * time.Second

type harness struct {
	t     *testing.T
	st    *synctest.Store
	gm    *synctest.Gmail
	ev    *synctest.Events
	nt    *synctest.Notifier
	clk   *clock.Fake
	tk    *synctest.Tickers
	skips *skipLog
	s     *syncer.Scheduler
}

func newHarness(t *testing.T) *harness { return newHarnessWith(t, func(*synctest.Store) {}) }

// newHarnessWith は Scheduler を作る前に prep でフェイクのストアを調整できる newHarness。
func newHarnessWith(t *testing.T, prep func(*synctest.Store)) *harness {
	t.Helper()
	h := &harness{
		t: t, st: synctest.NewStore(), gm: synctest.NewGmail(), ev: &synctest.Events{}, nt: &synctest.Notifier{},
		clk: clock.NewFake(time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)), tk: &synctest.Tickers{},
		skips: &skipLog{count: map[string]int{}},
	}
	prep(h.st)
	h.s = syncer.New(syncer.Deps{
		Store: h.st, Gmail: h.gm, Notifier: h.nt, Events: h.ev, Clock: h.clk,
		Interval: time.Minute, NewTicker: h.tk.New, Logger: slog.New(h.skips),
	})
	t.Cleanup(h.s.Shutdown)
	return h
}

// account はアカウントと同期状態、Gmail のスクリプトを用意する。
func (h *harness) account(id string, notifyEnabled bool, st store.SyncState) {
	h.t.Helper()
	st.AccountID = id
	h.st.Mu.Lock()
	h.st.Accounts[id] = store.Account{ID: id, Email: id + "@example.com", DisplayName: "Acct " + id, NotifyEnabled: notifyEnabled, Status: store.StatusActive}
	h.st.Order = append(h.st.Order, id)
	h.st.States[id] = st
	h.st.Mu.Unlock()
	h.gm.Edit(id, func(*synctest.Script) {})
}

func synced(historyID string) store.SyncState {
	return store.SyncState{InitialSyncDone: true, HistoryID: historyID}
}

func (h *harness) waitStatus(id, status string, n int) {
	h.t.Helper()
	require.Eventually(h.t, func() bool { return h.ev.Count(id, status) >= n }, waitTimeout, 2*time.Millisecond,
		"account %s: status %q x%d (got %v)", id, status, n, h.ev.Statuses(id))
}

func (h *harness) waitActive(id string, n int) { h.t.Helper(); h.waitStatus(id, "active", n) }

func (h *harness) waitFor(cond func() bool, msg string) {
	h.t.Helper()
	require.Eventually(h.t, cond, waitTimeout, 2*time.Millisecond, msg)
}

// syncTicker は 60 秒周期のティッカーのうち idx 番目（作成順）を返す。
func (h *harness) syncTicker(idx int) *synctest.Ticker {
	h.t.Helper()
	var ts []*synctest.Ticker
	h.waitFor(func() bool { ts = h.tk.ByInterval(time.Minute); return len(ts) > idx }, "ticker created")
	return ts[idx]
}

// tickWhilePaused は paused 中のアカウントのワーカーへ tick を 1 回送り、ワーカーがそれを
// 見送って（処理し終えて）待機に戻るまで待つ。連続で呼んでも、次の呼び出しまでに前の tick が
// 後続の Start へ持ち越されることはない。
func (h *harness) tickWhilePaused(id string, tk *synctest.Ticker) {
	h.t.Helper()
	want := h.skips.skipped(id) + 1
	tk.Tick(1)
	h.waitFor(func() bool { return h.skips.skipped(id) >= want }, "worker skipped the tick while paused")
}

func msg(id, thread string, labels ...string) gmail.MessageMeta {
	return gmail.MessageMeta{
		ID: id, ThreadID: thread, LabelIDs: labels, Snippet: "snippet " + id,
		InternalDate: time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC),
		From:         model.Address{Name: "Alice", Email: "Alice@Example.com"},
		To:           []model.Address{{Email: "me@example.com"}},
		Cc:           []model.Address{{Name: "Bob", Email: "bob@example.com"}},
		Subject:      "Subject " + thread,
	}
}

func thread(id string, msgs ...gmail.MessageMeta) gmail.ThreadMeta {
	return gmail.ThreadMeta{ID: id, HistoryID: "th-" + id, Messages: msgs}
}

// snap は DB に事前投入するスナップショット。
func snap(account, thread string, msgs ...store.MessageSnapshot) store.ThreadSnapshot {
	return store.ThreadSnapshot{AccountID: account, ThreadID: thread, Subject: "Subject " + thread, Messages: msgs}
}

func ms(id string, labels ...string) store.MessageSnapshot {
	return store.MessageSnapshot{ID: id, FromEmail: "alice@example.com", LabelIDs: labels}
}

func stubs(ids ...string) []gmail.ThreadStub {
	out := make([]gmail.ThreadStub, 0, len(ids))
	for _, id := range ids {
		out = append(out, gmail.ThreadStub{ID: id})
	}
	return out
}
