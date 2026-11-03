package apptests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

const removeWait = 5 * time.Second

// gatedGmail は最初の ListHistory で止まり、release されるまで返らない（キャンセルは無視する）。
// 進行中の同期周期が、ワーカーの停止要求に応じず走り続けている状態を作る。
type gatedGmail struct {
	*synctest.Gmail
	once      sync.Once
	openOnce  sync.Once
	entered   chan struct{} // 最初の ListHistory に入ったら閉じる
	cancelled chan struct{} // その呼び出しの ctx がキャンセルされたら閉じる
	release   chan struct{}
}

func newGatedGmail() *gatedGmail {
	return &gatedGmail{
		Gmail: synctest.NewGmail(), entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{}),
	}
}

func (g *gatedGmail) ListHistory(ctx context.Context, accountID, token string) (gmail.HistoryPage, error) {
	g.once.Do(func() {
		close(g.entered)
		go func() {
			<-ctx.Done()
			close(g.cancelled)
		}()
		<-g.release
	})
	return g.Gmail.ListHistory(ctx, accountID, token)
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(removeWait):
		t.Fatalf("タイムアウト: %s", what)
	}
}

// 実スケジューラで周期の最中にアカウントを削除しても、削除済みアカウントへ書き込まず、
// 誤った error ステータスも通知も出ない。同期の停止と周期の終了待ちが DB 削除より先であることを確かめる。
func TestRemoveAccountStopsInFlightSyncBeforeDeleting(t *testing.T) {
	e := apptest.NewEnv(t)
	a := e.AddAccount(t, "me@a.example")
	ctx := t.Context()
	require.NoError(t, e.Store.SaveSyncState(ctx, store.SyncState{AccountID: a.ID, InitialSyncDone: true, HistoryID: "1"}))

	gm := newGatedGmail()
	gm.Edit(a.ID, func(s *synctest.Script) {
		s.History = gmail.HistoryPage{LatestHistoryID: "2", ChangedThreadIDs: []string{"t1"}}
		s.Threads["t1"] = gmail.ThreadMeta{ID: "t1", HistoryID: "h1", Messages: []gmail.MessageMeta{
			{ID: "m1", ThreadID: "t1", InternalDate: apptest.Epoch, LabelIDs: []string{"INBOX", "UNREAD"}},
		}}
	})
	sched := syncer.New(syncer.Deps{
		Store: e.Store, Gmail: gm, Notifier: e.Notifier, Events: e.Events, Clock: e.Clock,
		NewTicker: func(time.Duration) (<-chan time.Time, func()) { return make(chan time.Time), func() {} },
	})
	t.Cleanup(sched.Shutdown)
	t.Cleanup(gm.open) // 失敗時も周期を解放して Shutdown が詰まらないようにする（クリーンアップは後入れ先出し）
	e.Deps.Sync = sched
	svc := app.NewAccountService(e.Deps)

	sched.Start(a.ID)
	waitClosed(t, gm.entered, "同期周期が ListHistory に入る")

	removed := make(chan error, 1)
	go func() { removed <- svc.RemoveAccount(ctx, a.ID) }()

	// 同期の停止が要求された（周期の ctx がキャンセルされた）時点では、まだ DB を消していない。
	waitClosed(t, gm.cancelled, "同期ワーカーが停止を要求される")
	_, err := e.Store.GetAccount(ctx, a.ID)
	require.NoError(t, err, "進行中の周期が終わるまで DeleteAccount しない")
	select {
	case err := <-removed:
		t.Fatalf("周期の終了前に RemoveAccount が返った: %v", err)
	default:
	}

	gm.open()
	select {
	case err := <-removed:
		require.NoError(t, err)
	case <-time.After(removeWait):
		t.Fatal("タイムアウト: RemoveAccount が返らない")
	}

	_, err = e.Store.GetAccount(ctx, a.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
	for _, ev := range e.Events.Named(events.AccountStatus) {
		p, ok := ev.Payload.(events.AccountStatusPayload)
		require.True(t, ok)
		assert.NotEqual(t, store.StatusError, p.Status, "削除中の周期が誤った error を出さない")
	}
	assert.Empty(t, e.Notifier.Notices, "削除済みアカウントの通知を出さない")
}

// failingDelete は DeleteAccount だけ失敗させる store.Store。
type failingDelete struct{ store.Store }

func (failingDelete) DeleteAccount(context.Context, string) error {
	return errors.New("disk I/O error")
}

func TestRemoveAccountRestartsSyncWhenDeleteFails(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	e.Deps.Store = failingDelete{e.Store}

	err := svc.RemoveAccount(t.Context(), a.ID)

	_ = requireCode(t, apperr.Internal, err)
	assert.Equal(t, []string{a.ID}, e.Sync.StoppedAccounts(), "先に同期を止める")
	assert.Equal(t, []string{a.ID}, e.Sync.StartedAccounts(), "削除に失敗したら同期を再開する")
	_, err = e.Store.GetAccount(t.Context(), a.ID)
	assert.NoError(t, err, "アカウントは残る")
	tok, _ := e.Secrets.Get(secrets.RefreshTokenKey(a.ID))
	assert.NotEmpty(t, tok, "資格情報は消さない")
}

// open は ListHistory を解放する（複数回呼んでよい）。
func (g *gatedGmail) open() { g.openOnce.Do(func() { close(g.release) }) }
