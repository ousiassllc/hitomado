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
