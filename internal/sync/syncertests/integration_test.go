package syncertests

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

// failingStore は実 SQLite の Store を包み、指定スレッドの ApplyThread を失敗させる。
type failingStore struct {
	store.Store
	mu     sync.Mutex
	failOn map[string]bool
}

func (f *failingStore) ApplyThread(ctx context.Context, s store.ThreadSnapshot) (store.ApplyResult, error) {
	f.mu.Lock()
	fail := f.failOn[s.ThreadID]
	f.mu.Unlock()
	if fail {
		return store.ApplyResult{}, errors.New("injected apply failure")
	}
	return f.Store.ApplyThread(ctx, s)
}

func (f *failingStore) setFail(threadID string, v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failOn[threadID] = v
}

// realHarness は実 SQLite（temp ファイル）で Scheduler を駆動する。
type realHarness struct {
	t    *testing.T
	st   *failingStore
	gm   *synctest.Gmail
	ev   *synctest.Events
	nt   *synctest.Notifier
	tk   *synctest.Tickers
	s    *syncer.Scheduler
	acct string
}

func newRealHarness(t *testing.T) *realHarness {
	t.Helper()
	clk := clock.NewFake(time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC))
	real, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "h.db"), store.WithClock(clk))
	require.NoError(t, err)
	t.Cleanup(func() { _ = real.Close() })
	oc := store.OAuthClient{ID: store.NewID(), Name: "client", ClientID: "cid"}
	require.NoError(t, real.CreateOAuthClient(t.Context(), oc))
	a := store.Account{
		ID: store.NewID(), OAuthClientID: oc.ID, Email: "me@example.com", DisplayName: "Me",
		Color: "red", NotifyEnabled: true, Status: store.StatusActive,
	}
	require.NoError(t, real.CreateAccount(t.Context(), a))

	h := &realHarness{
		t: t, st: &failingStore{Store: real, failOn: map[string]bool{}}, gm: synctest.NewGmail(),
		ev: &synctest.Events{}, nt: &synctest.Notifier{}, tk: &synctest.Tickers{}, acct: a.ID,
	}
	h.s = syncer.New(syncer.Deps{
		Store: h.st, Gmail: h.gm, Notifier: h.nt, Events: h.ev, Clock: clk,
		Interval: time.Minute, NewTicker: h.tk.New,
	})
	t.Cleanup(h.s.Shutdown)
	h.gm.Edit(h.acct, func(s *synctest.Script) {
		s.Labels = []gmail.Label{{ID: "INBOX", Type: "system"}, {ID: "UNREAD", Type: "system"}, {ID: "DRAFT", Type: "system"}}
	})
	return h
}

func (h *realHarness) edit(fn func(s *synctest.Script)) { h.gm.Edit(h.acct, fn) }

// cycleDone は active／error のどちらかの終了イベントが n 回に達するまで待つ。
func (h *realHarness) cycleDone(n int) {
	h.t.Helper()
	require.Eventually(h.t, func() bool {
		return h.ev.Count(h.acct, "active")+h.ev.Count(h.acct, "error") >= n
	}, waitTimeout, 2*time.Millisecond, "cycle %d (statuses %v)", n, h.ev.Statuses(h.acct))
}

func (h *realHarness) tick() {
	h.t.Helper()
	var ts []*synctest.Ticker
	require.Eventually(h.t, func() bool { ts = h.tk.ByInterval(time.Minute); return len(ts) > 0 }, waitTimeout, 2*time.Millisecond)
	ts[0].Tick(1)
}

func (h *realHarness) inbox() []store.ThreadRow {
	h.t.Helper()
	rows, err := h.st.ListInbox(h.t.Context(), store.InboxFilter{Limit: 50})
	require.NoError(h.t, err)
	return rows
}

func (h *realHarness) historyID() string {
	h.t.Helper()
	st, err := h.st.GetSyncState(h.t.Context(), h.acct)
	require.NoError(h.t, err)
	return st.HistoryID
}

func (h *realHarness) noticeThreads() []string {
	out := []string{}
	for _, n := range h.nt.Got() {
		out = append(out, n.ThreadID)
	}
	return out
}

func threadIDs(rows []store.ThreadRow) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, r.ThreadID)
	}
	return out
}

// initialInbox は t1（未読）・t3（既読）で初回同期を済ませる。
func (h *realHarness) initialInbox() {
	h.edit(func(s *synctest.Script) {
		s.ProfileHist = "100"
		s.Pages[""] = gmail.ThreadList{Threads: stubs("t1", "t3")}
		s.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX", "UNREAD"))
		s.Threads["t3"] = thread("t3", msg("m3", "t3", "INBOX"))
	})
	h.s.Start(h.acct)
	h.cycleDone(1)
	require.Equal(h.t, "100", h.historyID())
	require.Empty(h.t, h.nt.Got(), "初回同期は通知しない")
}

func TestRealStoreDeltaAppliesAddedLabelChangedAndDeleted(t *testing.T) {
	h := newRealHarness(t)
	h.initialInbox()
	assert.ElementsMatch(t, []string{"t1", "t3"}, threadIDs(h.inbox()))

	h.edit(func(s *synctest.Script) {
		s.History = gmail.HistoryPage{
			LatestHistoryID:  "110",
			ChangedThreadIDs: []string{"t1", "t2", "t3"},
			Added:            []gmail.HistoryAdded{{MessageID: "m2", ThreadID: "t2", LabelIDs: []string{"INBOX", "UNREAD"}}},
		}
		s.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX")) // 既読にした
		s.Threads["t2"] = thread("t2", msg("m2", "t2", "INBOX", "UNREAD"))
		delete(s.Threads, "t3") // Gmail 側で削除
	})
	h.tick()
	h.cycleDone(2)

	assert.ElementsMatch(t, []string{"t1", "t2"}, threadIDs(h.inbox()), "t3 は削除、t2 は追加")
	t1, err := h.st.GetThread(t.Context(), h.acct, "t1")
	require.NoError(t, err)
	assert.False(t, t1.IsUnread, "ラベル変更を反映")
	_, err = h.st.GetThread(t.Context(), h.acct, "t3")
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.Equal(t, "110", h.historyID())
	assert.Equal(t, []string{"t2"}, h.noticeThreads())
	counts, err := h.st.UnreadCounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[string]int{h.acct: 1}, counts)
	unread, _ := h.nt.LastUnread()
	assert.True(t, unread)
}

func TestRealStorePartialFailureKeepsHistoryAndRetriesNotice(t *testing.T) {
	h := newRealHarness(t)
	h.initialInbox()
	h.edit(func(s *synctest.Script) {
		s.History = gmail.HistoryPage{
			LatestHistoryID:  "120",
			ChangedThreadIDs: []string{"t4", "t5"},
			Added: []gmail.HistoryAdded{
				{MessageID: "m4", ThreadID: "t4", LabelIDs: []string{"INBOX", "UNREAD"}},
				{MessageID: "m5", ThreadID: "t5", LabelIDs: []string{"INBOX", "UNREAD"}},
			},
		}
		s.Threads["t4"] = thread("t4", msg("m4", "t4", "INBOX", "UNREAD"))
		s.Threads["t5"] = thread("t5", msg("m5", "t5", "INBOX", "UNREAD"))
	})
	h.st.setFail("t5", true)
	h.tick()
	h.cycleDone(2)

	assert.Equal(t, 1, h.ev.Count(h.acct, "error"))
	assert.Equal(t, "100", h.historyID(), "途中失敗では history_id を進めない")
	assert.Equal(t, []string{"t4"}, h.noticeThreads(), "反映済みの t4 は失敗周期でも通知する")
	acct, err := h.st.GetAccount(t.Context(), h.acct)
	require.NoError(t, err)
	assert.Equal(t, store.StatusError, acct.Status)

	h.st.setFail("t5", false)
	h.tick()
	h.cycleDone(3)
	assert.Equal(t, "120", h.historyID())
	assert.Equal(t, []string{"t4", "t5"}, h.noticeThreads(), "再試行で t5 だけが通知され、t4 は二重に出ない")
	assert.ElementsMatch(t, []string{"t1", "t3", "t4", "t5"}, threadIDs(h.inbox()))
}

func TestRealStoreExcludesDrafts(t *testing.T) {
	h := newRealHarness(t)
	h.initialInbox()
	draft := msg("d1", "t1", "DRAFT")
	draft.From.Email = "me@example.com"
	draft.InternalDate = draft.InternalDate.Add(time.Hour) // 最新になっても行を先頭に上げない
	h.edit(func(s *synctest.Script) {
		s.History = gmail.HistoryPage{
			LatestHistoryID:  "130",
			ChangedThreadIDs: []string{"t1", "t6"},
			Added: []gmail.HistoryAdded{
				{MessageID: "d1", ThreadID: "t1", LabelIDs: []string{"DRAFT", "INBOX", "UNREAD"}},
				{MessageID: "d6", ThreadID: "t6", LabelIDs: []string{"DRAFT"}},
			},
		}
		s.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX", "UNREAD"), draft)
		s.Threads["t6"] = thread("t6", msg("d6", "t6", "DRAFT"))
	})
	h.tick()
	h.cycleDone(2)

	row, err := h.st.GetThread(t.Context(), h.acct, "t1")
	require.NoError(t, err)
	assert.Equal(t, 1, row.MessageCount, "下書きは数えない")
	assert.Equal(t, "alice@example.com", row.LastFrom.Email, "最新の差出人は下書き（自分）にならない")
	_, err = h.st.GetThread(t.Context(), h.acct, "t6")
	assert.ErrorIs(t, err, store.ErrNotFound, "下書きだけのスレッドは保存しない")
	assert.Empty(t, h.nt.Got(), "下書きは通知しない")
}

func TestRealStoreResetSync(t *testing.T) {
	h := newRealHarness(t)
	h.initialInbox()
	h.edit(func(s *synctest.Script) {
		s.HistoryErr = fmt.Errorf("history: %w", gmail.ErrHistoryExpired)
		s.ProfileHist = "900"
		s.Pages[""] = gmail.ThreadList{Threads: stubs("t1", "t7"), NextPageToken: "next"}
		s.Threads["t7"] = thread("t7", msg("m7", "t7", "INBOX", "UNREAD"))
	})
	h.tick()
	h.cycleDone(2)

	assert.ElementsMatch(t, []string{"t1", "t7"}, threadIDs(h.inbox()), "t3 は受信箱から外れる")
	assert.Empty(t, h.nt.Got(), "リセット同期は通知しない")
	assert.Equal(t, "900", h.historyID())
	st, err := h.st.GetSyncState(t.Context(), h.acct)
	require.NoError(t, err)
	assert.Equal(t, "next", st.InboxPageToken)
	assert.True(t, st.InitialSyncDone)
}
