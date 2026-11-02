package confirm

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

var t0 = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

const ttl = 5 * time.Minute

func newManager() (*Manager, *clock.Fake) {
	c := clock.NewFake(t0)
	return NewManager(c, ttl), c
}

func hashOf(subject string) Hash {
	return HashContent(ContentInput{AccountID: "a1", Subject: subject})
}

func TestIssueAndVerifySuccess(t *testing.T) {
	m, _ := newManager()
	h := hashOf("s")
	tok, exp := m.Issue("s1", h)
	assert.Len(t, string(tok), 32, "128bit hex")
	assert.Equal(t, t0.Add(ttl), exp)
	assert.NoError(t, m.Verify("s1", tok, h))
}

func TestIssueTokensAreRandom(t *testing.T) {
	m, _ := newManager()
	a, _ := m.Issue("s1", hashOf("s"))
	b, _ := m.Issue("s2", hashOf("s"))
	assert.NotEqual(t, a, b)
}

func TestVerifyFailures(t *testing.T) {
	h := hashOf("s")
	tests := []struct {
		name    string
		prepare func(m *Manager, c *clock.Fake) (sessionID string, tok Token)
		hash    Hash
		wantErr error
	}{
		{"空トークン", func(m *Manager, _ *clock.Fake) (string, Token) {
			m.Issue("s1", h)
			return "s1", ""
		}, h, ErrRequired},
		{"未知のトークン", func(m *Manager, _ *clock.Fake) (string, Token) {
			m.Issue("s1", h)
			return "s1", Token("deadbeef")
		}, h, ErrRequired},
		{"未発行のセッション", func(*Manager, *clock.Fake) (string, Token) {
			return "none", Token("deadbeef")
		}, h, ErrRequired},
		{"別セッションのトークン", func(m *Manager, _ *clock.Fake) (string, Token) {
			tok, _ := m.Issue("s1", h)
			m.Issue("s2", h)
			return "s2", tok
		}, h, ErrRequired},
		{"期限切れ（ちょうど TTL）", func(m *Manager, c *clock.Fake) (string, Token) {
			tok, _ := m.Issue("s1", h)
			c.Advance(ttl)
			return "s1", tok
		}, h, ErrExpired},
		{"期限切れ（TTL 超過）", func(m *Manager, c *clock.Fake) (string, Token) {
			tok, _ := m.Issue("s1", h)
			c.Advance(ttl + time.Second)
			return "s1", tok
		}, h, ErrExpired},
		{"使用済み", func(m *Manager, _ *clock.Fake) (string, Token) {
			tok, _ := m.Issue("s1", h)
			require.NoError(t, m.Verify("s1", tok, h))
			return "s1", tok
		}, h, ErrExpired},
		{"ハッシュ不一致", func(m *Manager, _ *clock.Fake) (string, Token) {
			tok, _ := m.Issue("s1", h)
			return "s1", tok
		}, hashOf("other"), ErrMismatch},
		{"期限切れはハッシュ不一致より優先", func(m *Manager, c *clock.Fake) (string, Token) {
			tok, _ := m.Issue("s1", h)
			c.Advance(ttl)
			return "s1", tok
		}, hashOf("other"), ErrExpired},
		{"Invalidate 後", func(m *Manager, _ *clock.Fake) (string, Token) {
			tok, _ := m.Issue("s1", h)
			m.Invalidate("s1")
			return "s1", tok
		}, h, ErrRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, c := newManager()
			sid, tok := tt.prepare(m, c)
			assert.ErrorIs(t, m.Verify(sid, tok, tt.hash), tt.wantErr)
		})
	}
}

func TestVerifyJustBeforeExpiry(t *testing.T) {
	m, c := newManager()
	h := hashOf("s")
	tok, _ := m.Issue("s1", h)
	c.Advance(ttl - time.Nanosecond)
	assert.NoError(t, m.Verify("s1", tok, h))
}

func TestMismatchDoesNotConsumeToken(t *testing.T) {
	m, _ := newManager()
	h := hashOf("s")
	tok, _ := m.Issue("s1", h)
	require.ErrorIs(t, m.Verify("s1", tok, hashOf("changed")), ErrMismatch)
	require.ErrorIs(t, m.Verify("s1", tok, hashOf("changed again")), ErrMismatch)
	assert.NoError(t, m.Verify("s1", tok, h), "内容を戻せば同じトークンで送れる")
	assert.ErrorIs(t, m.Verify("s1", tok, h), ErrExpired, "成功後は使用済み")
}

func TestReissueInvalidatesOldToken(t *testing.T) {
	m, _ := newManager()
	h := hashOf("s")
	old, _ := m.Issue("s1", h)
	fresh, _ := m.Issue("s1", h)
	require.NotEqual(t, old, fresh)
	assert.ErrorIs(t, m.Verify("s1", old, h), ErrRequired)
	assert.NoError(t, m.Verify("s1", fresh, h), "旧トークンの失敗は新トークンに影響しない")
}

func TestReissueAfterUseGivesUsableToken(t *testing.T) {
	m, _ := newManager()
	h := hashOf("s")
	tok, _ := m.Issue("s1", h)
	require.NoError(t, m.Verify("s1", tok, h))
	tok2, _ := m.Issue("s1", h)
	assert.NoError(t, m.Verify("s1", tok2, h))
}

func TestSessionsAreIndependent(t *testing.T) {
	m, _ := newManager()
	h := hashOf("s")
	t1, _ := m.Issue("s1", h)
	t2, _ := m.Issue("s2", h)
	m.Invalidate("s1")
	assert.NoError(t, m.Verify("s2", t2, h))
	assert.ErrorIs(t, m.Verify("s1", t1, h), ErrRequired)
}

func TestPruneKeepsRecentlyExpiredAndDropsOld(t *testing.T) {
	m, c := newManager()
	h := hashOf("s")
	tok, _ := m.Issue("s1", h)
	c.Advance(ttl + time.Minute)
	m.Issue("s2", h) // 他セッションの発行では期限直後のエントリを消さない
	assert.ErrorIs(t, m.Verify("s1", tok, h), ErrExpired)

	c.Advance(pruneGrace + time.Minute)
	m.Issue("s3", h)
	assert.ErrorIs(t, m.Verify("s1", tok, h), ErrRequired)
	m.mu.Lock()
	defer m.mu.Unlock()
	_, kept := m.entries["s1"]
	assert.False(t, kept)
}

func TestVerifyIsSingleUseUnderConcurrency(t *testing.T) {
	m, _ := newManager()
	h := hashOf("s")
	tok, _ := m.Issue("s1", h)

	const n = 32
	var wg sync.WaitGroup
	results := make(chan error, n)
	start := make(chan struct{})
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- m.Verify("s1", tok, h)
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		if err == nil {
			ok++
		} else {
			assert.ErrorIs(t, err, ErrExpired)
		}
	}
	assert.Equal(t, 1, ok)
}

func TestHashContent(t *testing.T) {
	base := ContentInput{
		AccountID:   "a1",
		To:          []model.Address{{Name: "Bob", Email: "bob@x.com"}, {Email: "carol@x.com"}},
		Cc:          []model.Address{{Email: "dan@x.com"}},
		Bcc:         []model.Address{{Email: "eve@x.com"}},
		Subject:     "件名",
		BodyHTML:    "<p>本文</p>",
		Attachments: []AttachmentRef{{ID: "a", Size: 1}, {ID: "b", Size: 2}},
		Archive:     true,
	}
	want := HashContent(base)

	same := base
	same.To = []model.Address{{Name: "Bob", Email: " BOB@x.com "}, {Email: "Carol@X.com"}}
	same.Attachments = []AttachmentRef{{ID: "b", Size: 2}, {ID: "a", Size: 1}}
	assert.Equal(t, want, HashContent(same), "大文字小文字・空白・添付順は無視")

	tests := []struct {
		name   string
		mutate func(c *ContentInput)
	}{
		{"送信元", func(c *ContentInput) { c.AccountID = "a2" }},
		{"To 順序", func(c *ContentInput) { c.To = []model.Address{c.To[1], c.To[0]} }},
		{"To 追加", func(c *ContentInput) {
			c.To = append(append([]model.Address{}, c.To...), model.Address{Email: "z@x.com"})
		}},
		{"To→Cc 移動", func(c *ContentInput) {
			c.Cc = append([]model.Address{c.To[1]}, c.Cc...)
			c.To = c.To[:1]
		}},
		{"Cc→Bcc 移動", func(c *ContentInput) { c.Bcc = append(c.Bcc, c.Cc...); c.Cc = nil }},
		{"表示名", func(c *ContentInput) { c.To = []model.Address{{Name: "Rob", Email: "bob@x.com"}, c.To[1]} }},
		{"件名", func(c *ContentInput) { c.Subject += "!" }},
		{"本文", func(c *ContentInput) { c.BodyHTML += " " }},
		{"添付サイズ", func(c *ContentInput) { c.Attachments = []AttachmentRef{{ID: "a", Size: 9}, {ID: "b", Size: 2}} }},
		{"添付 ID", func(c *ContentInput) { c.Attachments = []AttachmentRef{{ID: "a", Size: 1}, {ID: "c", Size: 2}} }},
		{"添付削除", func(c *ContentInput) { c.Attachments = c.Attachments[:1] }},
		{"アーカイブ", func(c *ContentInput) { c.Archive = false }},
		{"件名と本文の境界", func(c *ContentInput) { c.Subject, c.BodyHTML = c.Subject+"<p>", "本文</p>" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base
			tt.mutate(&c)
			assert.NotEqual(t, want, HashContent(c))
		})
	}
}

func TestHashContentDoesNotMutateInput(t *testing.T) {
	in := ContentInput{Attachments: []AttachmentRef{{ID: "b"}, {ID: "a"}}}
	HashContent(in)
	assert.Equal(t, "b", in.Attachments[0].ID)
}
