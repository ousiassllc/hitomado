package agentcore

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

func TestPreviewCacheInvalidatesWhenDraftIDChanges(t *testing.T) {
	c := newPreviewCache()
	c.put("i1", "d1", "本文")
	got, ok := c.get("i1", "d1")
	assert.True(t, ok)
	assert.Equal(t, "本文", got)

	_, ok = c.get("i1", "d2")
	assert.False(t, ok, "下書きが作り直された（DraftID が変わった）ら使わない")
	_, ok = c.get("other", "d1")
	assert.False(t, ok)

	c.put("i1", "d2", "新しい本文")
	got, ok = c.get("i1", "d2")
	assert.True(t, ok)
	assert.Equal(t, "新しい本文", got)
}

func TestPreviewCacheIsBounded(t *testing.T) {
	c := newPreviewCache()
	for i := range previewCacheSize + 10 {
		c.put("i"+strconv.Itoa(i), "d", "x")
	}
	assert.Len(t, c.entries, previewCacheSize)
	_, ok := c.get("i0", "d")
	assert.False(t, ok, "古いものから捨てる")
	_, ok = c.get("i"+strconv.Itoa(previewCacheSize+9), "d")
	assert.True(t, ok)
}

func TestDraftLeadTextCutsSignatureAndQuote(t *testing.T) {
	d := gmail.DraftFull{Message: gmail.MessageFull{BodyHTML: `<p>こんにちは</p><div data-hitomado-signature data-account-id="a">-- <br>署名</div><blockquote data-hitomado-quote><p>引用</p></blockquote>`}}
	assert.Equal(t, "こんにちは", draftLeadText(d))

	d.Message.BodyHTML = `<blockquote data-hitomado-quote>引用</blockquote>`
	assert.Empty(t, draftLeadText(d))

	assert.Equal(t, "plain", draftLeadText(gmail.DraftFull{Message: gmail.MessageFull{BodyText: "plain"}}))
}
