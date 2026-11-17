// Package cooldown は、利用上限（429／レート制限の 403）に当たったアカウントの「この時刻まで待つ」を、並列の呼び出しで共有する。
package cooldown

import (
	"sync"
	"time"
)

// Cooldown は待ちの終わりの時刻。上限は分単位の枠なので、並列の呼び出しが別々に待って同時に投げ直すと、また当たる。
// 待ちの終わりは実時間で測る（待ち自体は gmail.Options.Sleep で差し替えられる）。複数の goroutine から使える。ゼロ値は「待たない」。
type Cooldown struct {
	mu    sync.Mutex
	until time.Time
}

// Remaining は残りの待ち時間（無ければ 0）。
func (c *Cooldown) Remaining() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return max(time.Until(c.until), 0)
}

// Extend は now+d までの待ちを記録する（すでにそれより先まで待つ記録があれば変えない）。
func (c *Cooldown) Extend(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if until := time.Now().Add(d); until.After(c.until) {
		c.until = until
	}
}
