package notify

import "sync"

// OpenRef は通知から開くスレッド。
type OpenRef struct {
	AccountID string
	ThreadID  string
}

// OpenRequests は「通知のタップで開くスレッド」の受け渡し口（Android）。
//
// タップはアプリのプロセス・画面（フロント）がまだ無いときにも起きる。フロントが購読を始める前に Go が notify:open-thread を
// 発行すると取りこぼすため、フロントが準備できた（Ready を呼んだ）あとだけ発行し、それまでは最新の 1 件を保持する
// （フロントは Ready の戻り値でそれを受け取る）。準備後に来た要求はそのまま deliver へ渡す。
type OpenRequests struct {
	deliver func(OpenRef)

	mu      sync.Mutex
	ready   bool
	pending *OpenRef
}

// NewOpenRequests は deliver（準備後の要求の発行先。notify:open-thread の発行）を持つ OpenRequests を返す。
func NewOpenRequests(deliver func(OpenRef)) *OpenRequests { return &OpenRequests{deliver: deliver} }

// Offer は開く要求を渡す。フロントが準備済みなら deliver し、そうでなければ最新の 1 件を保持する。
func (r *OpenRequests) Offer(ref OpenRef) {
	if ref.AccountID == "" || ref.ThreadID == "" {
		return
	}
	r.mu.Lock()
	if !r.ready {
		r.pending = &ref
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	r.deliver(ref)
}

// Ready はフロントの準備完了を記録し、それまでに保持した要求（無ければ nil）を返して空にする。
func (r *OpenRequests) Ready() *OpenRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ready = true
	p := r.pending
	r.pending = nil
	return p
}
