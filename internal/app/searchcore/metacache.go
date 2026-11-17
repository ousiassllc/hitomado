package searchcore

import (
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

// DefaultMetaCacheCapacity は MetaCache が覚えるスレッド数の目安。
const DefaultMetaCacheCapacity = 2000

type metaKey struct{ account, thread, history string }

// MetaCache は threads.get(format=metadata) の結果を (アカウント, スレッド, historyId) ごとに覚える（メモリのみ。ディスクには保存しない）。
// threads.list は各スレッドの現在の historyId を返すので、historyId が同じ＝スレッドは変わっていない。
// 検索の再実行・続きの読み込み・受信箱の読み直しで、同じスレッドの threads.get（10 units）を繰り返さないために使う。
// スレッドが変わる（返信・ラベル変更・既読など）と historyId が進むので、古い内容を返すことはない。複数の goroutine から使える。
type MetaCache struct {
	l *lru.Cache[metaKey, gmail.ThreadMeta]
}

// NewMetaCache は容量 capacity（0 以下は DefaultMetaCacheCapacity）のキャッシュを作る。
func NewMetaCache(capacity int) *MetaCache {
	if capacity <= 0 {
		capacity = DefaultMetaCacheCapacity
	}
	l, _ := lru.New[metaKey, gmail.ThreadMeta](capacity) // capacity > 0 のためエラーにならない
	return &MetaCache{l: l}
}

// get は historyID が一致する取得済みの結果を返す。historyID が空（不明）なら常に無し。nil の MetaCache は常に無し。
func (c *MetaCache) get(accountID, threadID, historyID string) (gmail.ThreadMeta, bool) {
	if c == nil || historyID == "" {
		return gmail.ThreadMeta{}, false
	}
	return c.l.Get(metaKey{accountID, threadID, historyID})
}

// put は取得した結果を、その結果自身の historyId で覚える。
func (c *MetaCache) put(accountID string, meta gmail.ThreadMeta) {
	if c == nil || meta.HistoryID == "" {
		return
	}
	c.l.Add(metaKey{accountID, meta.ID, meta.HistoryID}, meta)
}
