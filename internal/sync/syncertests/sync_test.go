package syncertests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

func TestInitialSyncStoresInboxWithoutNotifying(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, store.SyncState{})
	h.st.Mu.Lock()
	h.st.Accounts["a"] = store.Account{ID: "a", DisplayName: "A", NotifyEnabled: true, Status: store.StatusError}
	h.st.Mu.Unlock()
	h.gm.Edit("a", func(a *synctest.Script) {
		a.ProfileHist = "100"
		a.Pages[""] = gmail.ThreadList{Threads: stubs("t1", "t2", "t3"), NextPageToken: "p2"}
		for _, id := range []string{"t1", "t2", "t3"} {
			a.Threads[id] = thread(id, msg("m-"+id, id, "INBOX", "UNREAD"))
		}
		a.Labels = []gmail.Label{{ID: "INBOX", Type: "system"}, {ID: "UNREAD", Type: "system"}, {ID: "Label_1", Name: "仕事", Type: "user"}}
	})

	h.s.Start("a")
	h.waitActive("a", 1)

	assert.Empty(t, h.nt.Got(), "初回同期は通知しない")
	st := h.st.State("a")
	assert.Equal(t, "100", st.HistoryID)
	assert.Equal(t, "p2", st.InboxPageToken)
	assert.True(t, st.InitialSyncDone)
	assert.False(t, st.InboxExhausted)
	assert.False(t, st.LastSyncedAt.IsZero())
	assert.Equal(t, []string{"t1", "t2", "t3"}, h.st.ApplyOrder, "list の順に反映する")
	assert.Len(t, h.st.Labels["a"], 3)
	assert.Equal(t, []string{"syncing", "active"}, h.ev.Statuses("a"))
	assert.Equal(t, [][]string{{"t1", "t2", "t3"}}, h.ev.Updated("a"))
	assert.Equal(t, []string{"a:active:"}, h.st.StatusLog, "DB には syncing を保存せず、active のみ")
	unread, ok := h.nt.LastUnread()
	assert.True(t, ok)
	assert.True(t, unread)
	assert.Contains(t, h.st.Contacts["a"], model.Address{Name: "Alice", Email: "alice@example.com"})
	assert.Contains(t, h.st.Contacts["a"], model.Address{Name: "Bob", Email: "bob@example.com"})
	h.gm.Mu.Lock()
	defer h.gm.Mu.Unlock()
	assert.Equal(t, gmail.ListQuery{LabelIDs: []string{"INBOX"}, MaxResults: 200}, h.gm.Queries[0])
}

func TestInitialSyncLastPageSetsExhausted(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, store.SyncState{})
	h.gm.Edit("a", func(a *synctest.Script) {
		a.ProfileHist = "7"
		a.Pages[""] = gmail.ThreadList{Threads: stubs("t1")}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX"))
	})
	h.s.Start("a")
	h.waitActive("a", 1)
	st := h.st.State("a")
	assert.True(t, st.InboxExhausted)
	assert.Empty(t, st.InboxPageToken)
	unread, _ := h.nt.LastUnread()
	assert.False(t, unread)
}

func TestDeltaSyncAppliesAddedLabelChangedAndDeleted(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("100"))
	h.st.Put(snap("a", "t1", ms("m1", "INBOX", "UNREAD")))
	h.st.Put(snap("a", "t3", ms("m9", "INBOX")))
	h.gm.Edit("a", func(a *synctest.Script) {
		a.History = gmail.HistoryPage{
			LatestHistoryID:  "105",
			ChangedThreadIDs: []string{"t1", "t2", "t3"},
			Added:            []gmail.HistoryAdded{{MessageID: "m2", ThreadID: "t2", LabelIDs: []string{"INBOX", "UNREAD"}}},
		}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX")) // 既読になった（ラベル変更）
		a.Threads["t2"] = thread("t2", msg("m2", "t2", "INBOX", "UNREAD"))
		// t3 は Gmail 側で削除済み（ErrNotFound）
		a.Labels = []gmail.Label{{ID: "INBOX"}, {ID: "UNREAD"}}
	})
	h.st.Mu.Lock()
	h.st.Labels["a"] = []store.Label{{ID: "INBOX"}, {ID: "UNREAD"}}
	h.st.Mu.Unlock()

	h.s.Start("a")
	h.waitActive("a", 1)

	row, err := h.st.GetThread(t.Context(), "a", "t1")
	require.NoError(t, err)
	assert.False(t, row.IsUnread, "ラベル変更を反映")
	assert.True(t, h.st.Has("a", "t2"), "追加を反映")
	assert.False(t, h.st.Has("a", "t3"), "削除を反映")
	assert.Equal(t, []string{"a:t3"}, h.st.Deleted)
	assert.Equal(t, "105", h.st.State("a").HistoryID)
	assert.Equal(t, [][]string{{"t1", "t2", "t3"}}, h.ev.Updated("a"))
	assert.Equal(t, 0, h.gm.Count("a", "ListLabels"), "既知のラベルのみなら取り直さない")
	require.Len(t, h.nt.Got(), 1)
	n := h.nt.Got()[0]
	assert.Equal(t, "t2", n.ThreadID)
	assert.Equal(t, "Acct a", n.AccountName)
	assert.Equal(t, "Alice", n.FromName)
	assert.Equal(t, "Subject t2", n.Subject)
}

func TestDeltaSyncRefreshesLabelsWhenUnknownLabelAppears(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("1"))
	h.gm.Edit("a", func(a *synctest.Script) {
		a.History = gmail.HistoryPage{LatestHistoryID: "2", ChangedThreadIDs: []string{"t1"}}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX", "Label_9"))
		a.Labels = []gmail.Label{{ID: "INBOX"}, {ID: "Label_9", Name: "新規", Type: "user"}}
	})
	h.s.Start("a")
	h.waitActive("a", 1)
	assert.Equal(t, 1, h.gm.Count("a", "ListLabels"))
	assert.Len(t, h.st.Labels["a"], 2)
}

func TestDeltaSyncOnTickAndNoChangeEmitsNothing(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("1"))
	h.gm.Edit("a", func(a *synctest.Script) { a.History = gmail.HistoryPage{LatestHistoryID: "1"} })
	h.s.Start("a")
	h.waitActive("a", 1)
	h.syncTicker(0).Tick(1)
	h.waitActive("a", 2)
	assert.Equal(t, 2, h.gm.Count("a", "ListHistory"))
	assert.Empty(t, h.ev.Updated("a"), "変更なしでは sync:updated を出さない")
	_, ok := h.nt.LastUnread()
	assert.True(t, ok, "周期ごとにトレイの未読を更新する")
}

func TestResetSyncOnHistoryExpiredDoesNotNotify(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("old"))
	h.st.Put(snap("a", "gone", ms("m0", "INBOX")))
	h.gm.Edit("a", func(a *synctest.Script) {
		a.HistoryErr = fmt.Errorf("history: %w", gmail.ErrHistoryExpired)
		a.ProfileHist = "500"
		a.Pages[""] = gmail.ThreadList{Threads: stubs("t1", "t2"), NextPageToken: "next"}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX", "UNREAD"))
		a.Threads["t2"] = thread("t2", msg("m2", "t2", "INBOX", "UNREAD"))
		a.Labels = []gmail.Label{{ID: "INBOX"}, {ID: "UNREAD"}}
	})
	h.s.Start("a")
	h.waitActive("a", 1)

	assert.Empty(t, h.nt.Got(), "リセット同期は通知しない")
	assert.Equal(t, []string{"t1", "t2"}, h.st.Replaced["a"])
	assert.False(t, h.st.Has("a", "gone"))
	st := h.st.State("a")
	assert.Equal(t, "500", st.HistoryID)
	assert.Equal(t, "next", st.InboxPageToken)
	assert.True(t, st.InitialSyncDone)
	assert.Equal(t, [][]string{{"t1", "t2"}}, h.ev.Updated("a"))
	assert.Equal(t, []string{"syncing", "active"}, h.ev.Statuses("a"))
}

func TestResetSyncWhenHistoryIDMissing(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, store.SyncState{InitialSyncDone: true})
	h.gm.Edit("a", func(a *synctest.Script) {
		a.ProfileHist = "9"
		a.Pages[""] = gmail.ThreadList{Threads: stubs("t1")}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX", "UNREAD"))
	})
	h.s.Start("a")
	h.waitActive("a", 1)
	assert.Equal(t, 0, h.gm.Count("a", "ListHistory"))
	assert.Equal(t, []string{"t1"}, h.st.Replaced["a"])
	assert.Empty(t, h.nt.Got())
	assert.Equal(t, "9", h.st.State("a").HistoryID)
}

func TestNotificationRules(t *testing.T) {
	inboxUnread := []string{"INBOX", "UNREAD"}
	tests := []struct {
		name    string
		notify  bool
		pre     []store.MessageSnapshot // 事前に DB にあるメッセージ
		added   []gmail.HistoryAdded
		msgs    []gmail.MessageMeta
		wantIDs []string
	}{
		{
			name: "新規の未読 INBOX は通知", notify: true,
			added:   []gmail.HistoryAdded{{MessageID: "m1", ThreadID: "t1", LabelIDs: inboxUnread}},
			msgs:    []gmail.MessageMeta{msg("m1", "t1", inboxUnread...)},
			wantIDs: []string{"t1"},
		},
		{
			name: "通知オフのアカウントは出ない", notify: false,
			added: []gmail.HistoryAdded{{MessageID: "m1", ThreadID: "t1", LabelIDs: inboxUnread}},
			msgs:  []gmail.MessageMeta{msg("m1", "t1", inboxUnread...)},
		},
		{
			name: "INBOX でなければ出ない", notify: true,
			added: []gmail.HistoryAdded{{MessageID: "m1", ThreadID: "t1", LabelIDs: []string{"SENT", "UNREAD"}}},
			msgs:  []gmail.MessageMeta{msg("m1", "t1", "SENT", "UNREAD")},
		},
		{
			name: "既読は出ない", notify: true,
			added: []gmail.HistoryAdded{{MessageID: "m1", ThreadID: "t1", LabelIDs: []string{"INBOX"}}},
			msgs:  []gmail.MessageMeta{msg("m1", "t1", "INBOX")},
		},
		{
			name: "messagesAdded でない（ラベル変更のみ）は出ない", notify: true,
			pre:  []store.MessageSnapshot{ms("m1", "INBOX")},
			msgs: []gmail.MessageMeta{msg("m1", "t1", inboxUnread...)},
		},
		{
			name: "保存済みのメッセージは新規ではないので出ない", notify: true,
			pre:   []store.MessageSnapshot{ms("m1", "INBOX", "UNREAD")},
			added: []gmail.HistoryAdded{{MessageID: "m1", ThreadID: "t1", LabelIDs: inboxUnread}},
			msgs:  []gmail.MessageMeta{msg("m1", "t1", inboxUnread...)},
		},
		{
			name: "同一スレッドの複数の新着は 1 通知", notify: true,
			added: []gmail.HistoryAdded{
				{MessageID: "m1", ThreadID: "t1", LabelIDs: inboxUnread},
				{MessageID: "m2", ThreadID: "t1", LabelIDs: inboxUnread},
			},
			msgs:    []gmail.MessageMeta{msg("m1", "t1", inboxUnread...), msg("m2", "t1", inboxUnread...)},
			wantIDs: []string{"t1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.account("a", tt.notify, synced("1"))
			if len(tt.pre) > 0 {
				h.st.Put(snap("a", "t1", tt.pre...))
			}
			h.st.Mu.Lock()
			h.st.Labels["a"] = []store.Label{{ID: "INBOX"}, {ID: "UNREAD"}, {ID: "SENT"}}
			h.st.Mu.Unlock()
			h.gm.Edit("a", func(a *synctest.Script) {
				a.History = gmail.HistoryPage{LatestHistoryID: "2", ChangedThreadIDs: []string{"t1"}, Added: tt.added}
				a.Threads["t1"] = thread("t1", tt.msgs...)
			})
			h.s.Start("a")
			h.waitActive("a", 1)
			got := []string{}
			for _, n := range h.nt.Got() {
				got = append(got, n.ThreadID)
			}
			if tt.wantIDs == nil {
				tt.wantIDs = []string{}
			}
			assert.Equal(t, tt.wantIDs, got)
		})
	}
}
