package agentcore

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

var now = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)

func TestPreviewCacheInvalidatesWhenDraftIDChanges(t *testing.T) {
	c := newPreviewCache()
	c.put("i1", "d1", "本文")
	got, ok := c.get("i1", "d1", now)
	assert.True(t, ok)
	assert.Equal(t, "本文", got)

	_, ok = c.get("i1", "d2", now)
	assert.False(t, ok, "下書きが作り直された（DraftID が変わった）ら使わない")
	_, ok = c.get("other", "d1", now)
	assert.False(t, ok)

	c.put("i1", "d2", "新しい本文")
	got, ok = c.get("i1", "d2", now)
	assert.True(t, ok)
	assert.Equal(t, "新しい本文", got)
}

func TestPreviewCacheRemembersFailuresUntilRetryAt(t *testing.T) {
	c := newPreviewCache()
	c.putFailed("i1", "d1", now.Add(previewRetryAfter))
	got, ok := c.get("i1", "d1", now.Add(previewRetryAfter-time.Second))
	assert.True(t, ok, "失敗は一定時間覚えて、取り直さない")
	assert.Empty(t, got)
	_, ok = c.get("i1", "d1", now.Add(previewRetryAfter))
	assert.False(t, ok, "時間が過ぎたら取り直す")
	_, ok = c.get("i1", "d2", now)
	assert.False(t, ok, "DraftID が変わったら使わない")
}

func TestPreviewCacheIsBounded(t *testing.T) {
	c := newPreviewCache()
	for i := range previewCacheSize + 10 {
		c.put("i"+strconv.Itoa(i), "d", "x")
	}
	assert.Len(t, c.entries, previewCacheSize)
	_, ok := c.get("i0", "d", now)
	assert.False(t, ok, "古いものから捨てる")
	_, ok = c.get("i"+strconv.Itoa(previewCacheSize+9), "d", now)
	assert.True(t, ok)
}

func TestDraftLeadTextCutsSignatureAndQuote(t *testing.T) {
	d := gmail.DraftFull{Message: gmail.MessageFull{BodyHTML: `<p>こんにちは</p><div data-hitomado-signature data-account-id="a">-- <br>署名</div><blockquote data-hitomado-quote><p>引用</p></blockquote>`}}
	assert.Equal(t, "こんにちは", draftLeadText(d))

	d.Message.BodyHTML = `<blockquote data-hitomado-quote>引用</blockquote>`
	assert.Empty(t, draftLeadText(d))

	assert.Equal(t, "plain", draftLeadText(gmail.DraftFull{Message: gmail.MessageFull{BodyText: "plain"}}))
}
