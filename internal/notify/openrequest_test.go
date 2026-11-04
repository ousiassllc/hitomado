package notify

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type sink struct {
	mu   sync.Mutex
	refs []OpenRef
}

func (s *sink) deliver(r OpenRef) { s.mu.Lock(); s.refs = append(s.refs, r); s.mu.Unlock() }
func (s *sink) got() []OpenRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]OpenRef(nil), s.refs...)
}

func TestOpenRequestsHoldsUntilFrontendIsReady(t *testing.T) {
	s := &sink{}
	r := NewOpenRequests(s.deliver, nil)

	r.Offer(OpenRef{AccountID: "a", ThreadID: "t1"})
	r.Offer(OpenRef{AccountID: "a", ThreadID: "t2"})
	assert.Empty(t, s.got(), "フロントの準備前は発行しない（購読前に発行すると取りこぼす）")

	p := r.Ready()
	if assert.NotNil(t, p) {
		assert.Equal(t, OpenRef{AccountID: "a", ThreadID: "t2"}, *p, "保持するのは最新の 1 件")
	}
	assert.Nil(t, r.Ready(), "受け取ったら空になる")
	assert.Empty(t, s.got(), "Ready の戻り値で渡し、二重に発行しない")
}

func TestOpenRequestsDeliversAfterReady(t *testing.T) {
	s := &sink{}
	r := NewOpenRequests(s.deliver, nil)
	assert.Nil(t, r.Ready())
	r.Offer(OpenRef{AccountID: "a", ThreadID: "t1"})
	assert.Equal(t, []OpenRef{{AccountID: "a", ThreadID: "t1"}}, s.got())
	assert.Nil(t, r.Ready())
}

func TestOpenRequestsIgnoresIncompleteRefs(t *testing.T) {
	s := &sink{}
	r := NewOpenRequests(s.deliver, nil)
	r.Offer(OpenRef{AccountID: "a"})
	r.Offer(OpenRef{ThreadID: "t"})
	assert.Nil(t, r.Ready())
	r.Offer(OpenRef{})
	assert.Empty(t, s.got())
}

func TestNopBackground(t *testing.T) {
	var b Background = NopBackground{}
	b.ScheduleSync(true)
	b.ForgetAccount("a")
	b.RequestPermission()
	assert.Equal(t, PermissionUnsupported, b.Permission())
	assert.NoError(t, b.OpenSettings())
}

type javaSlot struct {
	mu  sync.Mutex
	ref *OpenRef
}

func (j *javaSlot) set(r OpenRef) { j.mu.Lock(); j.ref = &r; j.mu.Unlock() }
func (j *javaSlot) pull() *OpenRef {
	j.mu.Lock()
	defer j.mu.Unlock()
	r := j.ref
	j.ref = nil
	return r
}

func TestOpenRequestsColdStartPullsJavaSlotOnReady(t *testing.T) {
	// Java が起動時の Intent の要求を保持している（Go のイベントは起動前で取りこぼされた）。
	s, j := &sink{}, &javaSlot{}
	r := NewOpenRequests(s.deliver, j.pull)
	j.set(OpenRef{AccountID: "a", ThreadID: "t1"})

	p := r.Ready()
	if assert.NotNil(t, p) {
		assert.Equal(t, OpenRef{AccountID: "a", ThreadID: "t1"}, *p)
	}
	assert.Nil(t, r.Ready())
	assert.Empty(t, s.got())
}

func TestOpenRequestsWakeBeforeAndAfterReady(t *testing.T) {
	s, j := &sink{}, &javaSlot{}
	r := NewOpenRequests(s.deliver, j.pull)

	// フロントの準備前に起動済みのプロセスへタップが来た: 保持して、Ready で渡す（取りこぼさない）。
	j.set(OpenRef{AccountID: "a", ThreadID: "t1"})
	r.Wake()
	assert.Empty(t, s.got())
	p := r.Ready()
	if assert.NotNil(t, p) {
		assert.Equal(t, "t1", p.ThreadID)
	}

	// 準備後はそのまま発行する。
	j.set(OpenRef{AccountID: "a", ThreadID: "t2"})
	r.Wake()
	assert.Equal(t, []OpenRef{{AccountID: "a", ThreadID: "t2"}}, s.got())

	r.Wake() // 何も無ければ何もしない
	assert.Len(t, s.got(), 1)
}

func TestParseOpenRef(t *testing.T) {
	assert.Equal(t, &OpenRef{AccountID: "a", ThreadID: "t"}, ParseOpenRef(`{"accountId":"a","threadId":"t"}`))
	assert.Nil(t, ParseOpenRef(""))
	assert.Nil(t, ParseOpenRef("not json"))
	assert.Nil(t, ParseOpenRef(`{"accountId":"a"}`))
	assert.Nil(t, ParseOpenRef(`{"threadId":"t"}`))
}
