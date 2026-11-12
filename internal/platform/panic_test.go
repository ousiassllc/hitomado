package platform

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/events"
)

// recordingLogger は書かれた内容を buf に集める slog.Logger を返す。
func recordingLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

// fn が panic しても、プロセスは落ちず、WaitBounded は戻り、panic はスタック付きでログに残る。
func TestWaitBoundedRecoversPanic(t *testing.T) {
	t.Parallel()
	log, buf := recordingLogger()
	assert.True(t, WaitBounded(log, func() { panic("boom") }, time.Minute))
	assert.Contains(t, buf.String(), "msg=panic")
	assert.Contains(t, buf.String(), "boom")
	assert.Contains(t, buf.String(), "stack=")
}

// quit が panic しても、プロセスは落ちず、ログに残り、次の終了要求を受け付ける（待機が解ける）。
func TestQuitterRecoversPanicInQuitAndAcceptsNextRequest(t *testing.T) {
	t.Parallel()
	log, buf := recordingLogger()
	em := &recordEmitter{names: make(chan string, 4)}
	called := make(chan struct{}, 4)
	timer := make(chan time.Time, 1)
	q := NewQuitter(log, em, func() { called <- struct{}{}; panic("quit exploded") }, func(time.Duration) <-chan time.Time { return timer })

	q.Request()
	require.Equal(t, events.BeforeQuit, <-em.names)
	q.Ack()
	<-called
	require.Eventually(t, func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		return q.pending == nil
	}, 10*time.Second, time.Millisecond, "panic 後に待機が解けない")
	assert.Contains(t, buf.String(), "msg=panic")
	assert.Contains(t, buf.String(), "quit exploded")

	q.Request() // 次の要求を受け付ける
	assert.Equal(t, events.BeforeQuit, <-em.names)
}

// WindowKeeper の goroutine（restore・タイマーの Flush）が panic しても、プロセスは落ちずログに残る。
// ウィンドウ・アプリを持たない WindowKeeper で呼ぶと nil 参照で panic する。
func TestWindowKeeperGoroutinesRecoverPanic(t *testing.T) {
	t.Parallel()
	log, buf := recordingLogger()
	k := &WindowKeeper{log: log, saved: stateOf(Rect{X: 1, Y: 2, W: 800, H: 600}, false)}

	k.restore()
	assert.Contains(t, buf.String(), "window restore")
	buf.Reset()
	k.flushRecovered()
	assert.Contains(t, buf.String(), "window state flush")
	assert.Contains(t, buf.String(), "msg=panic")
}
