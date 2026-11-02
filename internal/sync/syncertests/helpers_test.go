package syncertests

import (
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

const waitTimeout = 5 * time.Second

type harness struct {
	t   *testing.T
	st  *synctest.Store
	gm  *synctest.Gmail
	ev  *synctest.Events
	nt  *synctest.Notifier
	clk *clock.Fake
	tk  *synctest.Tickers
	s   *syncer.Scheduler
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		t: t, st: synctest.NewStore(), gm: synctest.NewGmail(), ev: &synctest.Events{}, nt: &synctest.Notifier{},
		clk: clock.NewFake(time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)), tk: &synctest.Tickers{},
	}
	h.s = syncer.New(syncer.Deps{
		Store: h.st, Gmail: h.gm, Notifier: h.nt, Events: h.ev, Clock: h.clk,
		Interval: time.Minute, NewTicker: h.tk.New,
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
