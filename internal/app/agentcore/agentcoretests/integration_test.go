package agentcoretests

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
)

// 実 SQLite のストア・実 Scheduler・FakeGmail・フェイクの AI で、「新着が届く → 同期 → エージェントが下書きを作る」を通す。
// 初回同期で取り込んだ未読（今回の新着ではない）は扱わず、差分同期で届いた新着だけを扱う。送信は 1 度もしない。
func TestNewMailThroughSyncCreatesDraft(t *testing.T) {
	f := newFixture(t)
	f.enable(t)

	sched := syncer.New(syncer.Deps{
		Store: f.Store, Gmail: f.Gmail, Notifier: f.Notifier, Events: f.Events, Clock: f.Clock, Interval: time.Hour,
	})
	t.Cleanup(sched.Shutdown)
	sched.SetNewMailObserver(f.svc.Agent.OnNewMail)

	// 初回同期の対象: 既存の受信箱は t1 の 1 通（fixture が置いたもの）。同期の状態をまっさらにするため、アカウント b は使わない。
	f.Gmail.Profiles[f.a.ID] = gmail.Profile{Email: f.a.Email, HistoryID: "100"}
	f.Gmail.Labels[f.a.ID] = []gmail.Label{
		{ID: "INBOX", Name: "INBOX", Type: "system"}, {ID: "UNREAD", Name: "UNREAD", Type: "system"},
		{ID: "DRAFT", Name: "DRAFT", Type: "system"}, {ID: "SENT", Name: "SENT", Type: "system"},
		{ID: "CATEGORY_PROMOTIONS", Name: "CATEGORY_PROMOTIONS", Type: "system"},
	}
	sched.Start(f.a.ID)
	f.waitSyncCycles(t, 1)
	assert.Empty(t, f.items(t).Items, "初回同期で取り込んだメールは新着として扱わない")
	judges, _ := f.ai.counts()
	require.Zero(t, judges)

	// 新着 2 通: 返信が要るメインの受信箱の 1 通（n1）と、プロモーション（n2。対象外）。
	now := f.Clock.Now()
	n1 := apptest.Msg("n1-m1", "n1", now, "INBOX", "UNREAD")
	n1.From = model.Address{Name: secretFrom, Email: "bob@x.example"}
	n1.Subject = secretSubject
	n2 := apptest.Msg("n2-m1", "n2", now, "INBOX", "UNREAD", "CATEGORY_PROMOTIONS")
	f.Gmail.Threads[apptest.Key(f.a.ID, "n1")] = gmail.ThreadFull{ID: "n1", HistoryID: "h-n1", Messages: []gmail.MessageFull{n1}}
	f.Gmail.Threads[apptest.Key(f.a.ID, "n2")] = gmail.ThreadFull{ID: "n2", HistoryID: "h-n2", Messages: []gmail.MessageFull{n2}}
	f.Gmail.History[f.a.ID] = gmail.HistoryPage{
		LatestHistoryID:  "110",
		ChangedThreadIDs: []string{"n1", "n2"},
		Added: []gmail.HistoryAdded{
			{MessageID: "n1-m1", ThreadID: "n1", LabelIDs: n1.LabelIDs},
			{MessageID: "n2-m1", ThreadID: "n2", LabelIDs: n2.LabelIDs},
		},
	}
	sched.Trigger([]string{f.a.ID})
	f.waitItems(t, 1)

	it := f.only(t)
	assert.Equal(t, "n1", it.ThreadID)
	assert.Equal(t, "ready", it.Status)
	assert.Equal(t, f.a.ID, it.AccountID)
	assert.Equal(t, []model.Address{{Name: secretFrom, Email: "bob@x.example"}}, it.To)
	d, err := f.Gmail.GetDraft(t.Context(), f.a.ID, it.DraftID)
	require.NoError(t, err)
	body := d.Message.BodyHTML
	require.Contains(t, body, secretDraft)
	assert.Less(t, strings.Index(body, secretDraft), strings.Index(body, "署名A"), "本文 → 署名 の順: %s", body)
	assert.Equal(t, 1, f.Gmail.CallCount("CreateDraft"))
	judges, generates := f.ai.counts()
	assert.Equal(t, 1, judges, "プロモーションは AI に見せない")
	assert.Equal(t, 1, generates)

	updated := f.Events.Named(events.AgentUpdated)
	require.NotEmpty(t, updated)
	payload, ok := updated[len(updated)-1].Payload.(events.AgentUpdatedPayload)
	require.True(t, ok)
	assert.Equal(t, 1, payload.PendingCount)
	f.requireNeverSent(t)
}

// 同期が回っているあいだに、エージェントをオフにしておけば、新着が来ても何も作らない（同期は今までどおり進む）。
func TestNewMailThroughSyncWhileOffCreatesNothing(t *testing.T) {
	f := newFixture(t)
	f.ready(t) // オフのまま
	f.start(t)

	sched := syncer.New(syncer.Deps{
		Store: f.Store, Gmail: f.Gmail, Notifier: f.Notifier, Events: f.Events, Clock: f.Clock, Interval: time.Hour,
	})
	t.Cleanup(sched.Shutdown)
	sched.SetNewMailObserver(f.svc.Agent.OnNewMail)
	f.Gmail.Profiles[f.a.ID] = gmail.Profile{Email: f.a.Email, HistoryID: "100"}
	f.Gmail.Labels[f.a.ID] = []gmail.Label{{ID: "INBOX", Name: "INBOX", Type: "system"}, {ID: "UNREAD", Name: "UNREAD", Type: "system"}}
	sched.Start(f.a.ID)
	f.waitSyncCycles(t, 1)

	n1 := apptest.Msg("n1-m1", "n1", f.Clock.Now(), "INBOX", "UNREAD")
	f.Gmail.Threads[apptest.Key(f.a.ID, "n1")] = gmail.ThreadFull{ID: "n1", HistoryID: "h-n1", Messages: []gmail.MessageFull{n1}}
	f.Gmail.History[f.a.ID] = gmail.HistoryPage{
		LatestHistoryID: "110", ChangedThreadIDs: []string{"n1"},
		Added: []gmail.HistoryAdded{{MessageID: "n1-m1", ThreadID: "n1", LabelIDs: n1.LabelIDs}},
	}
	sched.Trigger([]string{f.a.ID})
	f.waitSyncCycles(t, 2)

	st, err := f.Store.GetSyncState(t.Context(), f.a.ID)
	require.NoError(t, err)
	assert.Equal(t, "110", st.HistoryID, "同期は進んだ")
	judges, generates := f.ai.counts()
	assert.Zero(t, judges)
	assert.Zero(t, generates)
	assert.Zero(t, f.Gmail.CallCount("CreateDraft"))
	assert.Empty(t, f.items(t).Items)
	f.requireNeverSent(t)
}

// waitSyncCycles は a アカウントの account:status の active が累計 n 件になるまで待つ（1 周期が終わるたびに 1 件）。
func (f *fixture) waitSyncCycles(t *testing.T, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		count := 0
		for _, e := range f.Events.Named(events.AccountStatus) {
			if p, ok := e.Payload.(events.AccountStatusPayload); ok && p.AccountID == f.a.ID && p.Status == "active" {
				count++
			}
		}
		return count >= n
	}, waitTimeout, 2*time.Millisecond, "同期の周期を %d 回待てなかった", n)
}
