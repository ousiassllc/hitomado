package headless

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

var (
	inboxUnread = []string{"INBOX", "UNREAD"}
	testClock   = clock.NewFake(time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC))
)

func newEnv(t *testing.T) (*synctest.Store, *synctest.Gmail) {
	t.Helper()
	return synctest.NewStore(), synctest.NewGmail()
}

// addAccount はアカウント（同期済み・history_id=100）と Gmail のスクリプトを用意する。
func addAccount(st *synctest.Store, gm *synctest.Gmail, id, status string, notify bool) {
	st.Mu.Lock()
	st.Accounts[id] = store.Account{ID: id, Email: id + "@example.com", DisplayName: "Acct " + id, NotifyEnabled: notify, Status: status}
	st.Order = append(st.Order, id)
	st.States[id] = store.SyncState{AccountID: id, InitialSyncDone: true, HistoryID: "100"}
	st.Labels[id] = []store.Label{{ID: "INBOX"}, {ID: "UNREAD"}, {ID: "DRAFT"}}
	st.Mu.Unlock()
	gm.Edit(id, func(*synctest.Script) {})
}

func newMailScript(a *synctest.Script, thread, message string) {
	meta := gmail.MessageMeta{
		ID: message, ThreadID: thread, LabelIDs: inboxUnread, Snippet: "s", Subject: "件名 " + thread,
		InternalDate: time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC),
		From:         model.Address{Name: "山田", Email: "yamada@example.com"},
	}
	a.History = gmail.HistoryPage{
		LatestHistoryID:  "105",
		ChangedThreadIDs: []string{thread},
		Added:            []gmail.HistoryAdded{{MessageID: message, ThreadID: thread, LabelIDs: inboxUnread}},
	}
	a.Threads[thread] = gmail.ThreadMeta{ID: thread, HistoryID: "th-" + thread, Messages: []gmail.MessageMeta{meta}}
}

func TestSyncWithReturnsNewMailWithoutNotifying(t *testing.T) {
	st, gm := newEnv(t)
	addAccount(st, gm, "a", store.StatusActive, true)
	gm.Edit("a", func(s *synctest.Script) { newMailScript(s, "t1", "m1") })

	res, err := syncWith(context.Background(), st, gm, testClock, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	require.Len(t, res.NewMail, 1)
	n := res.NewMail[0]
	assert.Equal(t, "a", n.AccountID)
	assert.Equal(t, "a@example.com", n.AccountEmail, "チャネルの表示名に使うアドレスを返す")
	assert.Equal(t, "Acct a", n.AccountName)
	assert.Equal(t, "t1", n.ThreadID)
	assert.Equal(t, "山田", n.FromName)
	assert.Equal(t, "件名 t1", n.Subject)
	assert.Equal(t, []AccountResult{{ID: "a", Email: "a@example.com", Status: StatusOK}}, res.Accounts)
	assert.Equal(t, "105", st.State("a").HistoryID, "同期状態を進める")
}

func TestSyncWithDoesNotReturnTheSameMailTwice(t *testing.T) {
	st, gm := newEnv(t)
	addAccount(st, gm, "a", store.StatusActive, true)
	gm.Edit("a", func(s *synctest.Script) { newMailScript(s, "t1", "m1") })

	first, err := syncWith(context.Background(), st, gm, testClock, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	require.Len(t, first.NewMail, 1)

	// 同じ履歴をもう一度返しても（アプリ本体と Worker の二重同期を想定）、DB に反映済みなので新着にならない。
	testClock.Advance(time.Minute) // 手動更新の 5 秒抑制を過ぎる
	second, err := syncWith(context.Background(), st, gm, testClock, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	assert.Empty(t, second.NewMail)
}

func TestSyncWithRespectsNotifyDisabled(t *testing.T) {
	st, gm := newEnv(t)
	addAccount(st, gm, "a", store.StatusActive, false)
	gm.Edit("a", func(s *synctest.Script) { newMailScript(s, "t1", "m1") })

	res, err := syncWith(context.Background(), st, gm, testClock, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	assert.Empty(t, res.NewMail, "通知を切ったアカウントは新着を返さない")
	assert.Equal(t, StatusOK, res.Accounts[0].Status)
	assert.True(t, st.Has("a", "t1"), "同期自体は行う")
}

func TestSyncWithClassifiesFailuresPerAccount(t *testing.T) {
	st, gm := newEnv(t)
	addAccount(st, gm, "ok", store.StatusActive, true)
	addAccount(st, gm, "reauth", store.StatusActive, true)
	addAccount(st, gm, "offline", store.StatusActive, true)
	addAccount(st, gm, "rejected", store.StatusActive, true)
	addAccount(st, gm, "waiting", store.StatusNeedsReauth, true)
	gm.Edit("ok", func(s *synctest.Script) { newMailScript(s, "t1", "m1") })
	gm.Edit("reauth", func(s *synctest.Script) { s.HistoryErr = fmt.Errorf("x: %w", gmail.ErrNeedsReauth) })
	gm.Edit("offline", func(s *synctest.Script) { s.HistoryErr = fmt.Errorf("x: %w", gmail.ErrOffline) })
	gm.Edit("rejected", func(s *synctest.Script) { s.HistoryErr = fmt.Errorf("x: %w", gmail.ErrClientRejected) })

	res, err := syncWith(context.Background(), st, gm, testClock, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	got := map[string]AccountStatus{}
	for _, a := range res.Accounts {
		got[a.ID] = a.Status
	}
	assert.Equal(t, map[string]AccountStatus{
		"ok": StatusOK, "reauth": StatusNeedsReauth, "offline": StatusError, "rejected": StatusNeedsReauth, "waiting": StatusNeedsReauth,
	}, got)
	require.Len(t, res.NewMail, 1, "失敗したアカウントがあっても、成功したアカウントの新着は返す")
	assert.Equal(t, "ok", res.NewMail[0].AccountID)
	assert.Zero(t, gm.Count("waiting", "ListHistory"), "再接続待ちのアカウントは Gmail に問い合わせない")
	assert.Equal(t, store.StatusNeedsReauth, st.Account("reauth").Status, "再接続が必要な状態は DB にも残る（アプリ本体と同じ分類）")
}

func TestSyncWithNoAccountsDoesNothing(t *testing.T) {
	st, gm := newEnv(t)
	res, err := syncWith(context.Background(), st, gm, testClock, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	assert.Empty(t, res.NewMail)
	assert.Empty(t, res.Accounts)
}

func TestSyncWithOnlyReauthAccountsDoesNotRefreshAll(t *testing.T) {
	// Refresh に空の ID を渡すと「全アカウント」になる。再接続待ちしか無いときに Gmail へ問い合わせないこと。
	st, gm := newEnv(t)
	addAccount(st, gm, "waiting", store.StatusNeedsReauth, true)
	res, err := syncWith(context.Background(), st, gm, testClock, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	assert.Equal(t, []AccountResult{{ID: "waiting", Email: "waiting@example.com", Status: StatusNeedsReauth}}, res.Accounts)
	assert.Empty(t, gm.Calls)
}

func TestRefreshAllListFailure(t *testing.T) {
	_, err := RefreshAll(context.Background(), failingLister{}, nil)
	require.Error(t, err)
}

type failingLister struct{}

func (failingLister) ListAccounts(context.Context) ([]store.Account, error) {
	return nil, errors.New("db closed")
}

var _ refresher = (*syncer.Scheduler)(nil)
