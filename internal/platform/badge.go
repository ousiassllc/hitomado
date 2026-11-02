package platform

import "sync"

// unreadBadge はトレイの未読バッジの状態を持ち、状態の変化があったときだけ apply を呼ぶ。
// 同期ワーカーなど複数のゴルーチンから set が呼ばれても、判定から apply までを 1 区間にして直列化する。
type unreadBadge struct {
	apply func(has bool)

	mu   sync.Mutex
	last bool
}

func (b *unreadBadge) set(has bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if has == b.last {
		return
	}
	b.last = has
	b.apply(has)
}
