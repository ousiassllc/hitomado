package synctest

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/notify"
)

// Events は発行されたイベントを記録する。
type Events struct {
	mu   sync.Mutex
	list []emitted
}

type emitted struct {
	name    string
	payload any
}

func (e *Events) Emit(name string, payload any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, emitted{name, payload})
}

// statuses は account:status の Status を発行順に返す。
func (e *Events) Statuses(id string) []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []string{}
	for _, ev := range e.list {
		if p, ok := ev.payload.(events.AccountStatusPayload); ok && p.AccountID == id {
			out = append(out, p.Status)
		}
	}
	return out
}

func (e *Events) Count(id, status string) int {
	n := 0
	for _, s := range e.Statuses(id) {
		if s == status {
			n++
		}
	}
	return n
}

// updated は sync:updated の changedThreadIds を発行順に返す。
func (e *Events) Updated(id string) [][]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := [][]string{}
	for _, ev := range e.list {
		if p, ok := ev.payload.(events.SyncUpdatedPayload); ok && p.AccountID == id {
			out = append(out, p.ChangedThreadIDs)
		}
	}
	return out
}

// Notifier は通知とトレイ未読を記録する。
type Notifier struct {
	mu      sync.Mutex
	notices []notify.NewMailNotice
	unread  []bool
}

func (n *Notifier) NewMail(_ context.Context, x notify.NewMailNotice) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notices = append(n.notices, x)
	return nil
}

func (n *Notifier) SetUnread(v bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.unread = append(n.unread, v)
}

func (n *Notifier) Got() []notify.NewMailNotice {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.notices)
}

func (n *Notifier) LastUnread() (bool, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.unread) == 0 {
		return false, false
	}
	return n.unread[len(n.unread)-1], true
}

// Tickers は NewTicker の差し替え。テストが tick を手動で送る。
type Tickers struct {
	mu      sync.Mutex
	tickers []*Ticker
}

type Ticker struct {
	D       time.Duration
	Ch      chan time.Time
	Stopped atomic.Bool
}

func (f *Tickers) New(d time.Duration) (<-chan time.Time, func()) {
	t := &Ticker{D: d, Ch: make(chan time.Time, 1)}
	f.mu.Lock()
	f.tickers = append(f.tickers, t)
	f.mu.Unlock()
	return t.Ch, func() { t.Stopped.Store(true) }
}

func (f *Tickers) ByInterval(d time.Duration) []*Ticker {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*Ticker{}
	for _, t := range f.tickers {
		if t.D == d {
			out = append(out, t)
		}
	}
	return out
}

// tick は n 回送る。バッファが 1 のため、k+2 回目の送信が終わった時点で k 回目の tick の処理は完了している。
func (t *Ticker) Tick(n int) {
	for range n {
		t.Ch <- time.Time{}
	}
}
