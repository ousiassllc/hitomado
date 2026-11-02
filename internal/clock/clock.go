// Package clock は時刻の注入用インターフェースと実装を提供する（トークン期限・保持期間・バックオフをテスト可能にする）。
package clock

import (
	"sync"
	"time"
)

// Clock は現在時刻の供給元。
type Clock interface {
	Now() time.Time
}

// System は実時間。
type System struct{}

// Now は現在時刻を返す。
func (System) Now() time.Time { return time.Now() }

// Fake はテスト用の手動進行クロック。並行利用できる。
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake は t から始まる Fake を返す。
func NewFake(t time.Time) *Fake { return &Fake{now: t} }

// Now は現在の擬似時刻を返す。
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance は擬似時刻を d だけ進める。
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}
