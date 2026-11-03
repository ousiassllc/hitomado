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
func (s *sink) got() []OpenRef    { s.mu.Lock(); defer s.mu.Unlock(); return append([]OpenRef(nil), s.refs...) }

func TestOpenRequestsHoldsUntilFrontendIsReady(t *testing.T) {
	s := &sink{}
	r := NewOpenRequests(s.deliver)

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
	r := NewOpenRequests(s.deliver)
	assert.Nil(t, r.Ready())
	r.Offer(OpenRef{AccountID: "a", ThreadID: "t1"})
	assert.Equal(t, []OpenRef{{AccountID: "a", ThreadID: "t1"}}, s.got())
	assert.Nil(t, r.Ready())
}

func TestOpenRequestsIgnoresIncompleteRefs(t *testing.T) {
	s := &sink{}
	r := NewOpenRequests(s.deliver)
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
