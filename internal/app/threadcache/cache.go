// Package threadcache はスレッド本文のメモリ LRU（NF-S09。ディスクには保存しない）を提供する。
package threadcache

import (
	"context"
	"errors"
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
//
// 取得中に Invalidate されたスレッドの結果は、その取得の呼び出し元にだけ返し、キャッシュにも
// 後続の相乗りにも渡さない。相乗り先の取得が context の取り消し・期限切れで失敗したときは、
// 自分の ctx が生きていれば取得をやり直す。
func (c *Cache) Get(ctx context.Context, accountID, threadID string) (gmail.ThreadFull, error) {
	k := key{accountID, threadID}
	for {
		if v, ok := c.lru.Get(k); ok {
			return v, nil
		}
		c.mu.Lock()
		// 上の確認のあとに先行の取得が完了して登録された場合（flight は消えている）に二重に取得しないよう、ロック下でもう一度見る。
		if v, ok := c.lru.Get(k); ok {
			c.mu.Unlock()
			return v, nil
		}
		f, ok := c.flight[k]
		if !ok {
			break // c.mu を保持したまま、自分がリーダーになる。
		}
		c.mu.Unlock()
		select {
		case <-f.done:
		case <-ctx.Done():
			return gmail.ThreadFull{}, ctx.Err()
		}
		if isContextErr(f.err) && ctx.Err() == nil {
			continue // リーダーの ctx だけが終わった。自分の ctx で取り直す。
		}
		return f.val, f.err
	}
	f := &call{done: make(chan struct{})}
	c.flight[k] = f
	c.mu.Unlock()

	f.val, f.err = c.client.GetThreadFull(ctx, accountID, threadID)
	c.mu.Lock()
	if c.flight[k] == f { // 取得中に無効化されていなければ登録する。
		delete(c.flight, k)
		if f.err == nil {
			c.lru.Add(k, f.val)
		}
	}
	c.mu.Unlock()
	close(f.done)
	return f.val, f.err
}

func isContextErr(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// Invalidate は指定スレッドのキャッシュと進行中の取得を破棄する。
func (c *Cache) Invalidate(accountID string, threadIDs ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range threadIDs {
		k := key{accountID, id}
		c.lru.Remove(k)
		delete(c.flight, k)
	}
}

// InvalidateAccount は当該アカウントのキャッシュと進行中の取得をすべて破棄する（アカウント削除時）。
func (c *Cache) InvalidateAccount(accountID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range c.lru.Keys() {
		if k.account == accountID {
			c.lru.Remove(k)
		}
	}
	for k := range c.flight {
		if k.account == accountID {
			delete(c.flight, k)
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
