package syncertests

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

// 手動更新の goroutine が panic しても、Refresh は戻ってエラーを返し、ログに panic が残り、次の Refresh が再び実行される。
func TestRefreshRecoversPanicAndRunsAgain(t *testing.T) {
	h := newHarness(t)
	quietAccount(h, "a")
	panicNext := true
	h.gm.HistHook = func(string) { // Start していないので、呼ぶのは Refresh の goroutine だけ
		if panicNext {
			panicNext = false
			panic("history exploded")
		}
	}

	res := h.s.Refresh(t.Context(), []string{"a"})
	require.Len(t, res, 1)
	require.Error(t, res[0].Err)
	assert.Contains(t, res[0].Err.Error(), "history exploded")
	assert.Equal(t, 1, h.skips.panicCount())

	h.clk.Advance(6 * time.Second) // 5 秒の抑制を過ぎる
	res = h.s.Refresh(t.Context(), []string{"a"})
	require.Len(t, res, 1)
	assert.NoError(t, res[0].Err)
	assert.Equal(t, 1, h.gm.Count("a", "ListHistory"), "panic のあとも再び同期する（panic した 1 回目はフックで記録前に落ちる）")
}

// メタデータ取得の goroutine が panic しても、プロセスは落ちず、取得全体の失敗としてエラーを返し、ログに panic が残る。
func TestFetchThreadsRecoversPanicInWorker(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, store.SyncState{InitialSyncDone: true, HistoryID: "1", InboxPageToken: "p2"})
	h.gm.Edit("a", func(a *synctest.Script) {
		a.Pages["p2"] = gmail.ThreadList{Threads: stubs("t3", "t4")}
		a.Labels = []gmail.Label{{ID: "INBOX"}, {ID: "UNREAD"}}
	})
	h.gm.ThreadHook = func(_, threadID string) {
		if threadID == "t4" {
			panic("metadata exploded")
		}
	}

	_, err := h.s.FetchMore(t.Context(), "a")
	require.Error(t, err)
	assert.False(t, errors.Is(err, gmail.ErrNotFound))
	assert.Contains(t, err.Error(), "metadata exploded")
	assert.Equal(t, 1, h.skips.panicCount())
}

// 保持期間の掃除が panic しても、ループは止まらず、次の tick でも掃除する（panic はログに残る）。
func TestRetentionRecoversPanicAndKeepsRunning(t *testing.T) {
	h := newHarnessWith(t, func(st *synctest.Store) { close(st.PurgeCh) }) // 閉じたチャネルへの送信で purge が panic する
	h.waitFor(func() bool { return h.skips.panicCount() >= 1 }, "panic at start is logged")
	var tk *synctest.Ticker
	h.waitFor(func() bool {
		ts := h.tk.ByInterval(24 * time.Hour)
		if len(ts) == 0 {
			return false
		}
		tk = ts[0]
		return true
	}, "retention ticker")
	h.clk.Advance(24 * time.Hour)
	tk.Tick(1)
	h.waitFor(func() bool { return h.skips.panicCount() >= 2 }, "retention loop still runs after a panic")
}
