package platform

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnreadBadgeAppliesOnlyOnChange(t *testing.T) {
	var got []bool
	b := &unreadBadge{apply: func(has bool) { got = append(got, has) }}

	b.set(false) // 初期状態（未読なし）から変わらない
	b.set(true)
	b.set(true)
	b.set(false)
	b.set(false)

	assert.Equal(t, []bool{true, false}, got)
}

// 並行に set しても、apply は排他的に呼ばれ、直前と異なる値でだけ呼ばれる（-race で検出する）。
func TestUnreadBadgeConcurrentSet(t *testing.T) {
	var inApply atomic.Int32
	var overlapped atomic.Bool
	var applied []bool // apply は直列なので、ロックなしで追記してよい（競合すれば -race が報告する）
	b := &unreadBadge{apply: func(has bool) {
		if inApply.Add(1) != 1 {
			overlapped.Store(true)
		}
		runtime.Gosched()
		applied = append(applied, has)
		inApply.Add(-1)
	}}

	const workers = 16
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 200 {
				b.set((i+j)%2 == 0)
			}
		}()
	}
	wg.Wait()

	assert.False(t, overlapped.Load(), "apply が並行に呼ばれた")
	prev := false
	for _, v := range applied {
		assert.NotEqual(t, prev, v, "同じ状態への apply は呼ばない")
		prev = v
	}
	if len(applied) > 0 {
		assert.Equal(t, applied[len(applied)-1], b.last)
	}
}
