package notify

import (
	"encoding/json"
	"sync"
)

// OpenRef は通知から開くスレッド。
type OpenRef struct {
	AccountID string
	ThreadID  string
}

// OpenRequests は「通知のタップで開くスレッド」の受け渡し口（Android）。
//
// タップはアプリのプロセス・画面（フロント）がまだ無いときにも起きる。通知のタップを受け取るのは Java（MainActivity の
// onCreate／onNewIntent）で、Java は要求を 1 件だけ保持する（pull で取り出す）。Go が起動する前に Java が発行したイベントは
// 取りこぼされるため、取りこぼさない順序にする:
//   - コールドスタート: フロントが購読を始めたあと Ready を呼ぶ。Ready は、保持している要求（無ければ pull で Java の分）を返す。
//   - 起動済み: Java のイベントで Wake が呼ばれ、pull した要求を Offer する。フロントが準備済みなら deliver
//     （notify:open-thread の発行）、まだなら最新の 1 件を保持して、準備後の Ready の戻り値で渡す。
type OpenRequests struct {
	deliver func(OpenRef)
	pull    func() *OpenRef // Java が保持する要求を取り出す（取り出すと空になる）。nil は常に「なし」

	mu      sync.Mutex
	ready   bool
	pending *OpenRef
}

// NewOpenRequests は deliver（準備後の要求の発行先。notify:open-thread の発行）と pull（Java が保持する要求の取り出し。
// nil 可）を持つ OpenRequests を返す。
func NewOpenRequests(deliver func(OpenRef), pull func() *OpenRef) *OpenRequests {
	return &OpenRequests{deliver: deliver, pull: pull}
}

// Wake は Java からの「要求がある」の合図で呼ぶ。保持されている要求を取り出して Offer する。
func (r *OpenRequests) Wake() {
	if r.pull == nil {
		return
	}
	if ref := r.pull(); ref != nil {
		r.Offer(*ref)
	}
}

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

// Ready はフロントの準備完了を記録し、それまでに保持した要求（無ければ Java が保持する分。どちらも無ければ nil）を返して空にする。
func (r *OpenRequests) Ready() *OpenRef {
	r.mu.Lock()
	r.ready = true
	p := r.pending
	r.pending = nil
	r.mu.Unlock()
	if p == nil && r.pull != nil {
		p = r.pull()
	}
	return p
}

// ParseOpenRef は Java の takeOpenRequest の戻り値（要求が無ければ空、あれば {"accountId":"…","threadId":"…"}）を読む。
// 空・壊れた JSON・どちらかの ID が空なら nil。
func ParseOpenRef(value string) *OpenRef {
	if value == "" {
		return nil
	}
	var v struct {
		AccountID string `json:"accountId"`
		ThreadID  string `json:"threadId"`
	}
	if err := json.Unmarshal([]byte(value), &v); err != nil || v.AccountID == "" || v.ThreadID == "" {
		return nil
	}
	return &OpenRef{AccountID: v.AccountID, ThreadID: v.ThreadID}
}
