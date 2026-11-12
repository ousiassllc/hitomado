package platform

import (
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/events"
)

type recordEmitter struct{ names chan string }

func (e *recordEmitter) Emit(name string, _ any) { e.names <- name }

func newTestQuitter(t *testing.T) (q *Quitter, em *recordEmitter, quit chan struct{}, timer chan time.Time) {
	t.Helper()
	em = &recordEmitter{names: make(chan string, 4)}
	quit = make(chan struct{}, 4)
	timer = make(chan time.Time, 1)
	q = NewQuitter(slog.New(slog.DiscardHandler), em, func() { quit <- struct{}{} }, func(time.Duration) <-chan time.Time { return timer })
	return q, em, quit, timer
}

func TestQuitterQuitsAfterAck(t *testing.T) {
	q, em, quit, _ := newTestQuitter(t)
	q.Request()
	assert.Equal(t, events.BeforeQuit, <-em.names)
	select {
	case <-quit:
		t.Fatal("Ack 前に終了してはいけない")
	default:
	}
	q.Ack()
	select {
	case <-quit:
	case <-time.After(5 * time.Second):
		t.Fatal("Ack 後に終了しなかった")
	}
}

func TestQuitterQuitsAfterGraceTimeout(t *testing.T) {
	q, _, quit, timer := newTestQuitter(t)
	q.Request()
	timer <- time.Now()
	select {
	case <-quit:
	case <-time.After(5 * time.Second):
		t.Fatal("猶予切れで終了しなかった")
	}
}

func TestQuitterAckWithoutRequestDoesNotQuit(t *testing.T) {
	q, _, quit, _ := newTestQuitter(t)
	q.Ack()
	select {
	case <-quit:
		t.Fatal("要求なしの Ack で終了してはいけない")
	default:
	}
}

func TestQuitterRequestIsIdempotent(t *testing.T) {
	q, em, quit, _ := newTestQuitter(t)
	q.Request()
	q.Request()
	require.Equal(t, events.BeforeQuit, <-em.names)
	assert.Empty(t, em.names, "待機中の再要求ではイベントを重ねて発行しない")
	q.Ack()
	<-quit
	var n atomic.Int32
	select {
	case <-quit:
		n.Add(1)
	default:
	}
	assert.Zero(t, n.Load(), "終了は 1 回だけ")
}
