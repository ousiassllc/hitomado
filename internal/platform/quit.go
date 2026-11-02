package platform

import (
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/events"
)

// QuitGrace はフロントが未保存の下書きを保存して Ack するのを待つ上限。
const QuitGrace = 3 * time.Second

// Quitter は「終了」の調停をする。終了要求でフロントに events.BeforeQuit を発行し、
// フロントが保存を終えて Ack するか猶予が過ぎたら quit を呼ぶ（F-12 のアプリ終了時の保存）。
type Quitter struct {
	emit  events.Emitter
	quit  func()
	after func(time.Duration) <-chan time.Time

	mu      sync.Mutex
	pending chan struct{} // 終了待ちの間だけ非 nil
}

// NewQuitter は quit（実際の終了処理）と猶予タイマー（nil なら time.After）を受け取る。
func NewQuitter(emit events.Emitter, quit func(), after func(time.Duration) <-chan time.Time) *Quitter {
	if after == nil {
		after = time.After
	}
	return &Quitter{emit: emit, quit: quit, after: after}
}

// Request は終了を要求する。すでに待機中なら何もしない。
func (q *Quitter) Request() {
	q.mu.Lock()
	if q.pending != nil {
		q.mu.Unlock()
		return
	}
	ack := make(chan struct{})
	q.pending = ack
	q.mu.Unlock()

	q.emit.Emit(events.BeforeQuit, nil)
	timeout := q.after(QuitGrace)
	go func() {
		select {
		case <-ack:
		case <-timeout:
		}
		q.quit()
	}()
}

// Ack はフロントの保存完了通知。待機中でなければ何もしない（終了要求なしの呼び出しで終了しない）。
func (q *Quitter) Ack() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.pending == nil {
		return
	}
	select {
	case <-q.pending:
	default:
		close(q.pending)
	}
}
