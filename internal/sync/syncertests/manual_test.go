package syncertests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

func TestFetchMorePagesUntilExhausted(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, store.SyncState{InitialSyncDone: true, HistoryID: "1", InboxPageToken: "p2"})
	h.gm.Edit("a", func(a *synctest.Script) {
		a.Pages["p2"] = gmail.ThreadList{Threads: stubs("t3", "t4"), NextPageToken: "p3"}
		a.Pages["p3"] = gmail.ThreadList{Threads: stubs("t5")}
		for _, id := range []string{"t3", "t4", "t5"} {
			a.Threads[id] = thread(id, msg("m-"+id, id, "INBOX", "UNREAD"))
		}
		a.Labels = []gmail.Label{{ID: "INBOX"}, {ID: "UNREAD"}}
	})
	ctx := t.Context()

	added, err := h.s.FetchMore(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, 2, added)
	st := h.st.State("a")
	assert.Equal(t, "p3", st.InboxPageToken)
	assert.False(t, st.InboxExhausted)
	assert.Equal(t, "1", st.HistoryID, "history_id には触れない")
	assert.Equal(t, []string{"t3", "t4"}, h.st.ApplyOrder)
	assert.Empty(t, h.nt.Got(), "ページ取得では通知しない")
	assert.Empty(t, h.ev.Updated("a"))
	unread, ok := h.nt.LastUnread()
	assert.True(t, ok)
	assert.True(t, unread, "古い未読もトレイ表示へ反映する")
	h.gm.Mu.Lock()
	assert.Equal(t, gmail.ListQuery{LabelIDs: []string{"INBOX"}, PageToken: "p2", MaxResults: 100}, h.gm.Queries[0])
	h.gm.Mu.Unlock()

	added, err = h.s.FetchMore(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, 1, added)
	st = h.st.State("a")
	assert.True(t, st.InboxExhausted, "末尾で exhausted")
	assert.Empty(t, st.InboxPageToken)

	calls := h.gm.Count("a", "ListThreads")
	added, err = h.s.FetchMore(ctx, "a")
	require.NoError(t, err)
	assert.Zero(t, added)
	assert.Equal(t, calls, h.gm.Count("a", "ListThreads"), "exhausted 後は Gmail を呼ばない")
}

func TestFetchMoreWithoutTokenDoesNotRefetchFirstPage(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, store.SyncState{InitialSyncDone: true, HistoryID: "1"})
	h.account("b", true, store.SyncState{HistoryID: ""}) // 初回同期が未完了
	h.gm.Edit("a", func(a *synctest.Script) { a.Pages[""] = gmail.ThreadList{Threads: stubs("t1")} })

	added, err := h.s.FetchMore(t.Context(), "a")
	require.NoError(t, err)
	assert.Zero(t, added)
	assert.Zero(t, h.gm.Count("a", "ListThreads"))
	assert.True(t, h.st.State("a").InboxExhausted)

	added, err = h.s.FetchMore(t.Context(), "b")
	require.NoError(t, err)
	assert.Zero(t, added)
	assert.Zero(t, h.gm.Count("b", "ListThreads"))
	assert.False(t, h.st.State("b").InboxExhausted, "初回同期前は状態を変えない")
}

func TestFetchMoreFailureKeepsPageToken(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, store.SyncState{InitialSyncDone: true, HistoryID: "1", InboxPageToken: "p2"})
	h.gm.Edit("a", func(a *synctest.Script) {
		a.Pages["p2"] = gmail.ThreadList{Threads: stubs("t3"), NextPageToken: "p3"}
		a.ThreadErr["t3"] = fmt.Errorf("x: %w", gmail.ErrOffline)
	})
	_, err := h.s.FetchMore(t.Context(), "a")
	assert.ErrorIs(t, err, gmail.ErrOffline)
	assert.Equal(t, "p2", h.st.State("a").InboxPageToken)
}

func TestFetchMoreNeedsReauthMarksAccount(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, store.SyncState{InitialSyncDone: true, HistoryID: "1", InboxPageToken: "p2"})
	h.gm.Edit("a", func(a *synctest.Script) { a.ListErr = fmt.Errorf("x: %w", gmail.ErrNeedsReauth) })
	_, err := h.s.FetchMore(t.Context(), "a")
	assert.ErrorIs(t, err, gmail.ErrNeedsReauth)
	assert.Equal(t, store.StatusNeedsReauth, h.st.Account("a").Status)
	assert.Equal(t, []string{"needs_reauth"}, h.ev.Statuses("a"))
}

func TestRefreshThreadFoundAndGone(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("1"))
	h.st.Put(snap("a", "t1", ms("m1", "INBOX", "UNREAD")))
	h.st.Put(snap("a", "t2", ms("m2", "INBOX")))
	h.gm.Edit("a", func(a *synctest.Script) {
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX")) // 既読になった
		a.Labels = []gmail.Label{{ID: "INBOX"}}
	})
	h.st.Mu.Lock()
	h.st.Labels["a"] = []store.Label{{ID: "INBOX"}}
	h.st.Mu.Unlock()

	row, found, err := h.s.RefreshThread(t.Context(), "a", "t1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "t1", row.ThreadID)
	assert.False(t, row.IsUnread)
	unread, ok := h.nt.LastUnread()
	assert.True(t, ok)
	assert.False(t, unread, "操作後はトレイの未読表示も更新する")

	row, found, err = h.s.RefreshThread(t.Context(), "a", "t2") // Gmail 側に無い
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, store.ThreadRow{}, row)
	assert.False(t, h.st.Has("a", "t2"), "存在しなければ DB からも削除")
	assert.Equal(t, []string{"a:t2"}, h.st.Deleted)
}

func TestRefreshThreadError(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("1"))
	h.gm.Edit("a", func(a *synctest.Script) { a.ThreadErr["t1"] = fmt.Errorf("x: %w", gmail.ErrRateLimited) })
	_, found, err := h.s.RefreshThread(t.Context(), "a", "t1")
	assert.ErrorIs(t, err, gmail.ErrRateLimited)
	assert.False(t, found)
}
