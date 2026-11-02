package syncertests

import (
	"context"
	"fmt"
	"runtime"
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

// overlapProbe は「未読数の取得 → SetUnread」の区間が重なったことを検出する。
// UnreadCounts を呼んだ時点で区間が始まり、SetUnread で終わる。区間中に別の UnreadCounts が来たら重なり（古い値で上書きし得る）。
type overlapProbe struct {
	mu       sync.Mutex
	open     int
	overlaps int
	calls    []bool
}

type probedStore struct {
	*synctest.Store
	p *overlapProbe
}

func (s probedStore) UnreadCounts(ctx context.Context) (map[string]int, error) {
	counts, err := s.Store.UnreadCounts(ctx)
	s.p.mu.Lock()
	if s.p.open > 0 {
		s.p.overlaps++
	}
	s.p.open++
	s.p.mu.Unlock()
	// 取得後に他のワーカーを走らせ、直列化されていなければ追い越しが起きるようにする。
	for range 20 {
		runtime.Gosched()
	}
	return counts, err
}

type probedNotifier struct {
	*synctest.Notifier
	p *overlapProbe
}

func (n probedNotifier) SetUnread(v bool) {
	n.p.mu.Lock()
	n.p.open--
	n.p.calls = append(n.p.calls, v)
	n.p.mu.Unlock()
	n.Notifier.SetUnread(v)
}

func TestUpdateUnreadIsSerializedAcrossAccounts(t *testing.T) {
	const accounts = 16
	p := &overlapProbe{}
	st := synctest.NewStore()
	gm := synctest.NewGmail()
	nt := &synctest.Notifier{}
	tk := &synctest.Tickers{}
	ev := &synctest.Events{}
	s := syncer.New(syncer.Deps{
		Store: probedStore{Store: st, p: p}, Gmail: gm, Notifier: probedNotifier{Notifier: nt, p: p}, Events: ev,
		Clock: clock.NewFake(time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)), Interval: time.Minute, NewTicker: tk.New,
	})
	t.Cleanup(s.Shutdown)

	ids := make([]string, accounts)
	for i := range ids {
		id := fmt.Sprintf("a%02d", i)
		ids[i] = id
		st.Mu.Lock()
		st.Accounts[id] = store.Account{ID: id, Email: id + "@example.com", DisplayName: id, Status: store.StatusActive}
		st.Order = append(st.Order, id)
		st.States[id] = store.SyncState{AccountID: id}
		st.Mu.Unlock()
		labels := []string{"INBOX"}
		if i%2 == 0 {
			labels = append(labels, "UNREAD")
		}
		gm.Edit(id, func(a *synctest.Script) {
			a.ProfileHist = "1"
			a.Pages[""] = gmail.ThreadList{Threads: stubs("t")}
			a.Threads["t"] = thread("t", msg("m-"+id, "t", labels...))
		})
	}
	for _, id := range ids {
		s.Start(id)
	}
	for _, id := range ids {
		require.Eventually(t, func() bool { return ev.Count(id, "active") >= 1 }, waitTimeout, 2*time.Millisecond, "account %s active", id)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	assert.Zero(t, p.overlaps, "未読数の取得から SetUnread までに別の更新が割り込んだ")
	assert.Zero(t, p.open)
	require.NotEmpty(t, p.calls)
	counts, err := st.UnreadCounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, len(counts) > 0, p.calls[len(p.calls)-1], "最後の SetUnread は DB の未読有無と一致する")
}
