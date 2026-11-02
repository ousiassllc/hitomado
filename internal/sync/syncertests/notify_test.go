package syncertests

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

func knownLabels(h *harness, id string) {
	h.st.Mu.Lock()
	defer h.st.Mu.Unlock()
	h.st.Labels[id] = []store.Label{{ID: "INBOX"}, {ID: "UNREAD"}, {ID: "DRAFT"}}
}

func noticeThreads(h *harness) []string {
	out := []string{}
	for _, n := range h.nt.Got() {
		out = append(out, n.ThreadID)
	}
	return out
}

func TestNotificationSurvivesMidwayFailureWithoutDuplicates(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("100"))
	knownLabels(h, "a")
	inboxUnread := []string{"INBOX", "UNREAD"}
	h.gm.Edit("a", func(a *synctest.Script) {
		a.History = gmail.HistoryPage{
			LatestHistoryID:  "105",
			ChangedThreadIDs: []string{"t1", "t2"},
			Added: []gmail.HistoryAdded{
				{MessageID: "m1", ThreadID: "t1", LabelIDs: inboxUnread},
				{MessageID: "m2", ThreadID: "t2", LabelIDs: inboxUnread},
			},
		}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", inboxUnread...))
		a.Threads["t2"] = thread("t2", msg("m2", "t2", inboxUnread...))
	})
	h.st.Mu.Lock()
	h.st.ApplyErr["t2"] = errors.New("disk full")
	h.st.Mu.Unlock()

	h.s.Start("a")
	h.waitStatus("a", "error", 1)
	assert.Equal(t, []string{"t1"}, noticeThreads(h), "反映済みの t1 は失敗周期でも通知する")
	assert.Equal(t, "100", h.st.State("a").HistoryID)

	h.st.Mu.Lock()
	delete(h.st.ApplyErr, "t2")
	h.st.Mu.Unlock()
	h.syncTicker(0).Tick(1)
	h.waitActive("a", 1)
	assert.Equal(t, "105", h.st.State("a").HistoryID)
	assert.Equal(t, []string{"t1", "t2"}, noticeThreads(h), "再試行で t2 だけが通知され、t1 は二重に出ない")
}

func TestDraftsAreNotStoredNorNotified(t *testing.T) {
	h := newHarness(t)
	h.account("a", true, synced("1"))
	knownLabels(h, "a")
	draft := msg("d1", "t1", "DRAFT")
	draft.From.Email = "acct-a@example.com"
	draftOnly := msg("d2", "t2", "DRAFT", "INBOX", "UNREAD")
	h.gm.Edit("a", func(a *synctest.Script) {
		a.History = gmail.HistoryPage{
			LatestHistoryID:  "2",
			ChangedThreadIDs: []string{"t1", "t2"},
			Added: []gmail.HistoryAdded{
				{MessageID: "d1", ThreadID: "t1", LabelIDs: []string{"DRAFT"}},
				{MessageID: "d2", ThreadID: "t2", LabelIDs: []string{"DRAFT", "INBOX", "UNREAD"}},
			},
		}
		a.Threads["t1"] = thread("t1", msg("m1", "t1", "INBOX"), draft)
		a.Threads["t2"] = thread("t2", draftOnly)
	})
	h.s.Start("a")
	h.waitActive("a", 1)

	h.st.Mu.Lock()
	defer h.st.Mu.Unlock()
	require.Len(t, h.st.Threads["a"]["t1"].Messages, 1, "下書きは保存しない")
	assert.Equal(t, "m1", h.st.Threads["a"]["t1"].Messages[0].ID)
	assert.NotContains(t, h.st.Threads["a"], "t2", "下書きだけのスレッドは保存しない")
	assert.Empty(t, h.nt.Got(), "下書きは通知しない")
	for _, c := range h.st.Contacts["a"] {
		assert.NotEqual(t, "acct-a@example.com", c.Email)
	}
}
