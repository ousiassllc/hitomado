// Package threadcache はスレッド本文のメモリ LRU（NF-S09。ディスクには保存しない）を提供する。
package threadcache

import (
	"context"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

// DefaultCapacity は保持するスレッド数の目安（NF-S09）。
const DefaultCapacity = 50

type key struct{ account, thread string }

type call struct {
	done chan struct{}
	val  gmail.ThreadFull
	err  error
}

// Cache は gmail.Client の GetThreadFull の前段に置く LRU。同時要求は 1 本にまとめる。
type Cache struct {
	client gmail.Client
	lru    *lru.Cache[key, gmail.ThreadFull]
	mu     sync.Mutex
	flight map[key]*call
}

// New は容量 capacity（0 以下は DefaultCapacity）のキャッシュを作る。
func New(c gmail.Client, capacity int) *Cache {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	l, _ := lru.New[key, gmail.ThreadFull](capacity) // capacity > 0 のためエラーにならない
	return &Cache{client: c, lru: l, flight: map[key]*call{}}
}

// Get はキャッシュにあればそれを返し、無ければ Gmail から取得して保持する。
func (c *Cache) Get(ctx context.Context, accountID, threadID string) (gmail.ThreadFull, error) {
	k := key{accountID, threadID}
	if v, ok := c.lru.Get(k); ok {
		return v, nil
	}
	c.mu.Lock()
	if f, ok := c.flight[k]; ok {
		c.mu.Unlock()
		select {
		case <-f.done:
			return f.val, f.err
		case <-ctx.Done():
			return gmail.ThreadFull{}, ctx.Err()
		}
	}
	f := &call{done: make(chan struct{})}
	c.flight[k] = f
	c.mu.Unlock()

	f.val, f.err = c.client.GetThreadFull(ctx, accountID, threadID)
	if f.err == nil {
		c.lru.Add(k, f.val)
	}
	c.mu.Lock()
	delete(c.flight, k)
	c.mu.Unlock()
	close(f.done)
	return f.val, f.err
}

// Invalidate は指定スレッドのキャッシュを破棄する。
func (c *Cache) Invalidate(accountID string, threadIDs ...string) {
	for _, id := range threadIDs {
		c.lru.Remove(key{accountID, id})
	}
}

// InvalidateAccount は当該アカウントのキャッシュをすべて破棄する（アカウント削除時）。
func (c *Cache) InvalidateAccount(accountID string) {
	for _, k := range c.lru.Keys() {
		if k.account == accountID {
			c.lru.Remove(k)
		}
	}
}

// Emitter は sync:updated を受けて変更スレッドのキャッシュを破棄してから next へ転送する。
func (c *Cache) Emitter(next events.Emitter) events.Emitter {
	return invalidatingEmitter{c: c, next: next}
}

type invalidatingEmitter struct {
	c    *Cache
	next events.Emitter
}

func (e invalidatingEmitter) Emit(name string, payload any) {
	if p, ok := payload.(events.SyncUpdatedPayload); ok && name == events.SyncUpdated {
		e.c.Invalidate(p.AccountID, p.ChangedThreadIDs...)
	}
	if e.next != nil {
		e.next.Emit(name, payload)
	}
}
