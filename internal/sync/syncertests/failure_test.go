package syncertests

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

func TestNeedsReauthStopsOnlyThatAccount(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("1"))
	h.account("b", true, synced("1"))
	h.gm.Edit("a", func(a *synctest.Script) { a.HistoryErr = fmt.Errorf("refresh: %w", gmail.ErrNeedsReauth) })
	h.gm.Edit("b", func(a *synctest.Script) { a.History = gmail.HistoryPage{LatestHistoryID: "1"} })

	h.s.Start("a")
	h.waitStatus("a", "needs_reauth", 1)
	h.s.Start("b")
	h.waitActive("b", 1)
	assert.Equal(t, store.StatusNeedsReauth, h.st.Account("a").Status)
	assert.Equal(t, "再接続が必要です", h.st.Account("a").StatusMessage)
	assert.Equal(t, store.StatusActive, h.st.Account("b").Status)

	// 周期が来ても a は再試行せず、b は継続する（ティッカーは作成順 a, b）。
	tickA, tickB := h.syncTicker(0), h.syncTicker(1)
	tickB.Tick(1)
	h.waitActive("b", 2)
	tickA.Tick(3) // a のワーカーが 1 回目の tick を処理し終えるまで進める
	assert.Equal(t, 1, h.gm.Count("a", "ListHistory"), "needs_reauth は自動再試行しない")

	// 再接続（Start）で再開する。
	h.gm.Edit("a", func(a *synctest.Script) {
		a.HistoryErr = nil
		a.History = gmail.HistoryPage{LatestHistoryID: "1"}
	})
	h.s.Start("a")
	h.waitActive("a", 1)
	assert.Equal(t, store.StatusActive, h.st.Account("a").Status)
	assert.Equal(t, 2, h.gm.Count("a", "ListHistory"), "再開後に 1 回同期")
}

func TestMidwayFailureDoesNotAdvanceHistoryID(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("100"))
	h.gm.Edit("a", func(a *synctest.Script) {
		a.History = gmail.HistoryPage{LatestHistoryID: "105", ChangedThreadIDs: []string{"t1", "t2"}}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX"))
		a.Threads["t2"] = thread("t2", msg("m2", "t2", "INBOX"))
		a.Labels = []gmail.Label{{ID: "INBOX"}}
	})
	h.st.Mu.Lock()
	h.st.Labels["a"] = []store.Label{{ID: "INBOX"}}
	h.st.ApplyErr["t2"] = errors.New("disk full")
	h.st.Mu.Unlock()

	h.s.Start("a")
	h.waitStatus("a", "error", 1)
	st := h.st.State("a")
	assert.Equal(t, "100", st.HistoryID, "途中失敗では history_id を進めない")
	assert.Equal(t, "同期に失敗しました", st.LastError)
	assert.Equal(t, store.StatusError, h.st.Account("a").Status)
	assert.Equal(t, [][]string{{"t1"}}, h.ev.Updated("a"), "反映済みの分は sync:updated に出す")

	// 次周期で再試行して成功する。
	h.st.Mu.Lock()
	delete(h.st.ApplyErr, "t2")
	h.st.Mu.Unlock()
	h.syncTicker(0).Tick(1)
	h.waitActive("a", 1)
	assert.Equal(t, "105", h.st.State("a").HistoryID)
	assert.Empty(t, h.st.State("a").LastError)
	assert.Equal(t, store.StatusActive, h.st.Account("a").Status)
}

func TestFetchFailureDoesNotAdvanceHistoryID(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("100"))
	h.gm.Edit("a", func(a *synctest.Script) {
		a.History = gmail.HistoryPage{LatestHistoryID: "105", ChangedThreadIDs: []string{"t1", "t2"}}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX"))
		a.ThreadErr["t2"] = fmt.Errorf("500: %w", gmail.ErrGeneric)
	})
	h.s.Start("a")
	h.waitStatus("a", "error", 1)
	assert.Equal(t, "100", h.st.State("a").HistoryID)
	assert.False(t, h.st.Has("a", "t1"), "取得に失敗したら何も反映しない")
	assert.Empty(t, h.ev.Updated("a"))
}

func TestScopeInsufficientStopsAccountWithGuidance(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("1"))
	// ErrNeedsReauth にも分類される形（gmail.APIError と同じ）。権限不足の案内が優先される。
	h.gm.Edit("a", func(a *synctest.Script) {
		a.HistoryErr = fmt.Errorf("x: %w", errors.Join(gmail.ErrNeedsReauth, gmail.ErrScopeInsufficient))
	})

	h.s.Start("a")
	h.waitStatus("a", "needs_reauth", 1)
	assert.Equal(t, store.StatusNeedsReauth, h.st.Account("a").Status)
	assert.Equal(t, "Gmail の権限（スコープ）が足りません。再接続して、すべての権限を許可してください", h.st.Account("a").StatusMessage)
}

func TestErrorClassesMapToStatusMessages(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{gmail.ErrOffline, "ネットワークに接続できません"},
		{gmail.ErrRateLimited, "Gmail のリクエスト制限中です。次の周期で再試行します"},
		{gmail.ErrGeneric, "同期に失敗しました"},
		{gmail.ErrAPIDisabled, "Google Cloud のプロジェクトで Gmail API が有効になっていません。有効にしてください"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			h := newHarness(t)
			h.account("a", true, synced("1"))
			h.gm.Edit("a", func(a *synctest.Script) { a.HistoryErr = fmt.Errorf("x: %w", tt.err) })
			h.s.Start("a")
			h.waitStatus("a", "error", 1)
			assert.Equal(t, tt.want, h.st.Account("a").StatusMessage)
			// 次の周期で復旧する。
			h.gm.Edit("a", func(a *synctest.Script) { a.HistoryErr = nil })
			h.syncTicker(0).Tick(1)
			h.waitActive("a", 1)
			assert.Equal(t, store.StatusActive, h.st.Account("a").Status)
		})
	}
}

func TestAccountsSyncInParallelAndFailuresAreIsolated(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("1"))
	h.account("b", true, synced("1"))
	entered, release := make(chan struct{}), make(chan struct{})
	h.gm.HistHook = func(id string) {
		if id == "a" {
			close(entered)
			<-release // a の遅延中も b は完了できる
		}
	}
	h.gm.Edit("a", func(a *synctest.Script) { a.History = gmail.HistoryPage{LatestHistoryID: "1"} })
	h.gm.Edit("b", func(a *synctest.Script) { a.History = gmail.HistoryPage{LatestHistoryID: "1"} })

	h.s.Start("a")
	<-entered
	h.s.Start("b")
	h.waitActive("b", 1)
	assert.Equal(t, 0, h.ev.Count("a", "active"), "a はまだ同期中")
	assert.Equal(t, []string{"syncing"}, h.ev.Statuses("a"))
	close(release)
	h.waitActive("a", 1)
}

func TestRefreshOfNeedsReauthAccountReturnsErrorWithoutCallingGmail(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("1"))
	h.gm.Edit("a", func(a *synctest.Script) { a.HistoryErr = gmail.ErrNeedsReauth })
	h.s.Start("a")
	h.waitStatus("a", "needs_reauth", 1)

	res := h.s.Refresh(t.Context(), []string{"a"})
	require.Len(t, res, 1)
	assert.ErrorIs(t, res[0].Err, gmail.ErrNeedsReauth)
	assert.Equal(t, 1, h.gm.Count("a", "ListHistory"))
}

// OAuth クライアントの資格情報が拒否されたとき（invalid_client 等）は、設定を確認する案内を出し、
// needs_reauth と同様に自動再試行を止める。再接続（Start）で再開する。
func TestClientRejectedStopsRetryingWithConfigMessage(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("1"))
	h.gm.Edit("a", func(a *synctest.Script) { a.HistoryErr = fmt.Errorf("refresh: %w", gmail.ErrClientRejected) })

	h.s.Start("a")
	h.waitStatus("a", "needs_reauth", 1)
	assert.Equal(t, store.StatusNeedsReauth, h.st.Account("a").Status)
	assert.Equal(t, "OAuth クライアントの設定（Client ID／Secret）を確認してください", h.st.Account("a").StatusMessage)
	assert.Equal(t, "OAuth クライアントの設定（Client ID／Secret）を確認してください", h.st.State("a").LastError)

	h.syncTicker(0).Tick(3) // ワーカーが tick を処理し終えるまで進める
	assert.Equal(t, 1, h.gm.Count("a", "ListHistory"), "自動再試行しない")

	h.gm.Edit("a", func(a *synctest.Script) {
		a.HistoryErr = nil
		a.History = gmail.HistoryPage{LatestHistoryID: "1"}
	})
	h.s.Start("a")
	h.waitActive("a", 1)
	assert.Equal(t, 2, h.gm.Count("a", "ListHistory"), "再開後に 1 回同期")
}
