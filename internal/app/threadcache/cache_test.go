package threadcache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

type countingClient struct {
	gmail.Client
	calls   atomic.Int32
	release chan struct{} // nil でなければ受信まで取得をブロックする
}

func (c *countingClient) GetThreadFull(_ context.Context, _, threadID string) (gmail.ThreadFull, error) {
	c.calls.Add(1)
	if c.release != nil {
		<-c.release
	}
	return gmail.ThreadFull{ID: threadID}, nil
}

type captureEmitter struct{ names []string }

func (e *captureEmitter) Emit(name string, _ any) { e.names = append(e.names, name) }

func TestGetCachesAndInvalidates(t *testing.T) {
	cl := &countingClient{}
	c := New(cl, 2)
	ctx := context.Background()

	_, err := c.Get(ctx, "a1", "t1")
	require.NoError(t, err)
	_, err = c.Get(ctx, "a1", "t1")
	require.NoError(t, err)
	assert.EqualValues(t, 1, cl.calls.Load(), "2 回目はキャッシュから返る")

	c.Invalidate("a1", "t1")
	_, _ = c.Get(ctx, "a1", "t1")
	assert.EqualValues(t, 2, cl.calls.Load(), "破棄後は再取得する")
}

func TestLRUEvictsOldest(t *testing.T) {
	cl := &countingClient{}
	c := New(cl, 2)
	ctx := context.Background()
	for _, id := range []string{"t1", "t2", "t3"} {
		_, _ = c.Get(ctx, "a1", id)
	}
	_, _ = c.Get(ctx, "a1", "t1")
	assert.EqualValues(t, 4, cl.calls.Load(), "容量超過で t1 が追い出されている")
}

func TestConcurrentRequestsShareOneFetch(t *testing.T) {
	cl := &countingClient{release: make(chan struct{})}
	c := New(cl, 2)
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Get(context.Background(), "a1", "t1")
		}()
	}
	// 先頭の取得が開始されるまで待ってから解放する。
	require.Eventually(t, func() bool { return cl.calls.Load() == 1 }, 5*time.Second, time.Millisecond)
	close(cl.release)
	wg.Wait()
	assert.EqualValues(t, 1, cl.calls.Load())
}

func TestInvalidatingEmitterDropsChangedThreads(t *testing.T) {
	cl := &countingClient{}
	c := New(cl, 5)
	ctx := context.Background()
	_, _ = c.Get(ctx, "a1", "t1")
	_, _ = c.Get(ctx, "a1", "t2")

	next := &captureEmitter{}
	c.Emitter(next).Emit(events.SyncUpdated, events.SyncUpdatedPayload{AccountID: "a1", ChangedThreadIDs: []string{"t1"}})
	assert.Equal(t, []string{events.SyncUpdated}, next.names, "イベントは転送される")

	_, _ = c.Get(ctx, "a1", "t1") // 再取得
	_, _ = c.Get(ctx, "a1", "t2") // キャッシュ
	assert.EqualValues(t, 3, cl.calls.Load())
}

func TestInvalidateAccount(t *testing.T) {
	cl := &countingClient{}
	c := New(cl, 5)
	ctx := context.Background()
	_, _ = c.Get(ctx, "a1", "t1")
	_, _ = c.Get(ctx, "a2", "t1")
	c.InvalidateAccount("a1")
	_, _ = c.Get(ctx, "a1", "t1")
	_, _ = c.Get(ctx, "a2", "t1")
	assert.EqualValues(t, 3, cl.calls.Load())
}

// gatedClient は呼び出しごとに started へ通知し、fn の結果を返す。
type gatedClient struct {
	gmail.Client
	calls   atomic.Int32
	started chan int32
	fn      func(ctx context.Context, n int32) (gmail.ThreadFull, error)
}

func (c *gatedClient) GetThreadFull(ctx context.Context, _, threadID string) (gmail.ThreadFull, error) {
	n := c.calls.Add(1)
	c.started <- n
	v, err := c.fn(ctx, n)
	v.ID = threadID
	return v, err
}

func waitStarted(t *testing.T, ch <-chan int32) int32 {
	t.Helper()
	select {
	case n := <-ch:
		return n
	case <-time.After(5 * time.Second):
		require.FailNow(t, "取得が開始されなかった")
		return 0
	}
}

func TestInvalidateDuringFetchDoesNotCacheOrShareStaleResult(t *testing.T) {
	release := make(chan struct{})
	cl := &gatedClient{started: make(chan int32, 4)}
	cl.fn = func(_ context.Context, n int32) (gmail.ThreadFull, error) {
		if n == 1 {
			<-release // 無効化前に始まった古い取得
		}
		return gmail.ThreadFull{HistoryID: map[int32]string{1: "stale", 2: "fresh"}[n]}, nil
	}
	c := New(cl, 2)
	ctx := context.Background()

	staleDone := make(chan gmail.ThreadFull, 1)
	go func() {
		v, _ := c.Get(ctx, "a1", "t1")
		staleDone <- v
	}()
	require.EqualValues(t, 1, waitStarted(t, cl.started))

	c.Invalidate("a1", "t1")

	// 無効化後の Get は古い取得に相乗りせず、新しく取得する（相乗りなら release まで返らず ctx 超過で失敗する）。
	freshCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	v, err := c.Get(freshCtx, "a1", "t1")
	require.NoError(t, err)
	assert.Equal(t, "fresh", v.HistoryID)
	require.EqualValues(t, 2, waitStarted(t, cl.started))

	close(release)
	<-staleDone

	// 古い取得の完了はキャッシュを上書きしない。
	v, err = c.Get(ctx, "a1", "t1")
	require.NoError(t, err)
	assert.Equal(t, "fresh", v.HistoryID)
	assert.EqualValues(t, 2, cl.calls.Load())
}

func TestInvalidateAccountDuringFetchDoesNotCacheStaleResult(t *testing.T) {
	release := make(chan struct{})
	cl := &gatedClient{started: make(chan int32, 4)}
	cl.fn = func(_ context.Context, n int32) (gmail.ThreadFull, error) {
		if n == 1 {
			<-release
		}
		return gmail.ThreadFull{}, nil
	}
	c := New(cl, 2)
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = c.Get(ctx, "a1", "t1")
	}()
	require.EqualValues(t, 1, waitStarted(t, cl.started))
	c.InvalidateAccount("a1")
	close(release)
	<-done

	_, err := c.Get(ctx, "a1", "t1")
	require.NoError(t, err)
	assert.EqualValues(t, 2, cl.calls.Load(), "無効化後に完了した取得は保持されず再取得する")
}

func TestLeaderCancelDoesNotPropagateToWaiter(t *testing.T) {
	cl := &gatedClient{started: make(chan int32, 4)}
	cl.fn = func(ctx context.Context, n int32) (gmail.ThreadFull, error) {
		if n == 1 {
			<-ctx.Done()
			return gmail.ThreadFull{}, ctx.Err()
		}
		return gmail.ThreadFull{HistoryID: "ok"}, nil
	}
	c := New(cl, 2)

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	leaderErr := make(chan error, 1)
	go func() {
		_, err := c.Get(leaderCtx, "a1", "t1")
		leaderErr <- err
	}()
	require.EqualValues(t, 1, waitStarted(t, cl.started))

	type result struct {
		v   gmail.ThreadFull
		err error
	}
	waiter := make(chan result, 1)
	go func() {
		v, err := c.Get(context.Background(), "a1", "t1")
		waiter <- result{v, err}
	}()
	// 相乗りしているなら新しい取得は始まらない。リーダーを取り消すと待機側が取り直す。
	cancelLeader()
	require.ErrorIs(t, <-leaderErr, context.Canceled)

	select {
	case r := <-waiter:
		require.NoError(t, r.err)
		assert.Equal(t, "ok", r.v.HistoryID)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "待機側が完了しなかった")
	}
}
