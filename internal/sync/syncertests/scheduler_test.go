package syncertests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

func quietAccount(h *harness, id string) {
	h.account(id, true, synced("1"))
	h.gm.Edit(id, func(a *synctest.Script) { a.History = gmail.HistoryPage{LatestHistoryID: "1"} })
}

func TestStartIsIdempotent(t *testing.T) {
	h := newHarness(t)
	quietAccount(h, "a")
	h.s.Start("a")
	h.s.Start("a")
	h.waitActive("a", 1)
	h.s.Start("a")
	assert.Len(t, h.tk.ByInterval(time.Minute), 1, "ワーカーは 1 つだけ")
	assert.Equal(t, 1, h.gm.Count("a", "ListHistory"))
}

func TestTriggerRunsASyncCycle(t *testing.T) {
	h := newHarness(t)
	quietAccount(h, "a")
	quietAccount(h, "b")
	h.s.Start("a")
	h.s.Start("b")
	h.waitActive("a", 1)
	h.waitActive("b", 1)

	h.s.Trigger([]string{"a"})
	h.waitActive("a", 2)
	assert.Equal(t, 1, h.ev.Count("b", "active"), "指定していないアカウントは同期しない")

	h.s.Trigger(nil) // 空は全ワーカー
	h.waitActive("a", 3)
	h.waitActive("b", 2)
}

func TestStopEndsWorkerAndStartRestartsIt(t *testing.T) {
	h := newHarness(t)
	quietAccount(h, "a")
	h.s.Start("a")
	h.waitActive("a", 1)
	tk := h.syncTicker(0)

	h.s.Stop("a")
	assert.True(t, tk.Stopped.Load(), "Stop はワーカーを終了させる")
	h.s.Stop("a") // 冪等
	h.s.Trigger([]string{"a"})
	assert.Equal(t, 1, h.gm.Count("a", "ListHistory"))

	h.s.Start("a")
	h.waitActive("a", 2)
	assert.Len(t, h.tk.ByInterval(time.Minute), 2)
}

func TestRefreshSuppressedWithinFiveSeconds(t *testing.T) {
	h := newHarness(t)
	quietAccount(h, "a")
	quietAccount(h, "b")
	h.s.Start("a")
	h.waitActive("a", 1)

	ctx := t.Context()
	first := h.s.Refresh(ctx, []string{"a"})
	require.Len(t, first, 1)
	assert.NoError(t, first[0].Err)
	assert.Equal(t, "a", first[0].AccountID)
	assert.Equal(t, 2, h.gm.Count("a", "ListHistory"))

	h.clk.Advance(4 * time.Second)
	second := h.s.Refresh(ctx, []string{"a"})
	assert.Equal(t, first, second, "5 秒未満は直近の結果を返す")
	assert.Equal(t, 2, h.gm.Count("a", "ListHistory"), "再同期しない")

	// 抑制はアカウントごと。b は同期され、空指定は全アカウント。
	all := h.s.Refresh(ctx, nil)
	require.Len(t, all, 2)
	assert.Equal(t, 1, h.gm.Count("b", "ListHistory"))
	assert.Equal(t, 2, h.gm.Count("a", "ListHistory"))

	h.clk.Advance(2 * time.Second)
	h.s.Refresh(ctx, []string{"a"})
	assert.Equal(t, 3, h.gm.Count("a", "ListHistory"), "5 秒経過後は再同期する")
}

func TestRefreshRunsAccountsInParallelAndSharesInFlightCall(t *testing.T) {
	h := newHarness(t)
	quietAccount(h, "a")
	quietAccount(h, "b")
	entered, release := make(chan string, 4), make(chan struct{})
	h.gm.HistHook = func(id string) {
		entered <- id
		<-release
	}
	done := make(chan []syncer.RefreshResult, 2)
	go func() { done <- h.s.Refresh(context.Background(), []string{"a", "b"}) }()
	// 両アカウントが同時に Gmail 呼び出し中になる（直列なら 2 件目は来ない）。
	got := map[string]bool{<-entered: true, <-entered: true}
	assert.Equal(t, map[string]bool{"a": true, "b": true}, got)
	// 実行中の同じ手動更新は共有される。
	go func() { done <- h.s.Refresh(context.Background(), []string{"a"}) }()
	close(release)
	for range 2 {
		select {
		case res := <-done:
			for _, r := range res {
				assert.NoError(t, r.Err)
			}
		case <-time.After(waitTimeout):
			t.Fatal("Refresh did not return")
		}
	}
	assert.Equal(t, 1, h.gm.Count("a", "ListHistory"))
}

func TestRefreshHonoursCallerContext(t *testing.T) {
	h := newHarness(t)
	quietAccount(h, "a")
	entered, release := make(chan struct{}), make(chan struct{})
	h.gm.HistHook = func(string) {
		close(entered)
		<-release
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan []syncer.RefreshResult, 1)
	go func() { done <- h.s.Refresh(ctx, []string{"a"}) }()
	<-entered
	cancel()
	res := <-done
	require.Len(t, res, 1)
	assert.ErrorIs(t, res[0].Err, context.Canceled)
	close(release)
}

func TestShutdownStopsAllWorkers(t *testing.T) {
	h := newHarness(t)
	quietAccount(h, "a")
	quietAccount(h, "b")
	h.s.Start("a")
	h.s.Start("b")
	h.waitActive("a", 1)
	h.waitActive("b", 1)

	done := make(chan struct{})
	go func() { h.s.Shutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(waitTimeout):
		t.Fatal("Shutdown did not return")
	}
	for _, tk := range h.tk.ByInterval(time.Minute) {
		assert.True(t, tk.Stopped.Load(), "全ワーカーのティッカーが止まる")
	}
	for _, tk := range h.tk.ByInterval(24 * time.Hour) {
		assert.True(t, tk.Stopped.Load())
	}
	h.s.Shutdown() // 冪等

	h.s.Start("a") // Shutdown 後は起動しない
	res := h.s.Refresh(context.Background(), []string{"a"})
	require.Len(t, res, 1)
	assert.ErrorIs(t, res[0].Err, syncer.ErrClosed)
	_, err := h.s.FetchMore(context.Background(), "a")
	assert.ErrorIs(t, err, syncer.ErrClosed)
	assert.Equal(t, 1, h.gm.Count("a", "ListHistory"))
}

func TestShutdownCancelsInFlightSync(t *testing.T) {
	h := newHarness(t)
	quietAccount(h, "a")
	entered, release := make(chan struct{}), make(chan struct{})
	h.gm.HistHook = func(string) {
		close(entered)
		<-release
	}
	h.s.Start("a")
	<-entered
	done := make(chan struct{})
	go func() { h.s.Shutdown(); close(done) }()
	close(release)
	select {
	case <-done:
	case <-time.After(waitTimeout):
		t.Fatal("Shutdown did not wait for the worker")
	}
	assert.Equal(t, 0, h.ev.Count("a", "error"), "中断は error 状態にしない")
}

func TestRetentionRunsAtStartAndEveryDay(t *testing.T) {
	h := newHarness(t)
	wantCutoff := h.clk.Now().Add(-90 * 24 * time.Hour)
	select {
	case got := <-h.st.PurgeCh:
		assert.True(t, got.Equal(wantCutoff), "起動直後に now-90日")
	case <-time.After(waitTimeout):
		t.Fatal("no purge at start")
	}
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
	select {
	case got := <-h.st.PurgeCh:
		assert.True(t, got.Equal(wantCutoff.Add(24*time.Hour)))
	case <-time.After(waitTimeout):
		t.Fatal("no purge on tick")
	}
}
