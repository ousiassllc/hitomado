package deps

import "sync/atomic"

// Busy は「いま入れ替え（自動アップデート）をしてはいけない処理」の数を数える。送信・下書きの保存・下書きの削除が、
// 始めるときに Begin を呼び、返された関数を終わりに呼ぶ。nil の *Busy は常に「空き」で、Begin は何もしない関数を返す。
type Busy struct{ n atomic.Int64 }

// Begin は処理の開始を記録し、終了を記録する関数（1 回だけ呼ぶ）を返す。
func (b *Busy) Begin() (end func()) {
	if b == nil {
		return func() {}
	}
	b.n.Add(1)
	return func() { b.n.Add(-1) }
}

// Active は処理が 1 つでも進行中か。
func (b *Busy) Active() bool { return b != nil && b.n.Load() > 0 }
