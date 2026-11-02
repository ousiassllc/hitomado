package mimeparse

import (
	"mime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gmailapi "google.golang.org/api/gmail/v1"
)

func h(name, value string) *gmailapi.MessagePartHeader {
	return &gmailapi.MessagePartHeader{Name: name, Value: value}
}

func TestParseAddressList(t *testing.T) {
	got := ParseAddressList(`=?UTF-8?B?5bGx55Sw?= <Yamada@Example.COM>, "Doe, John" <john@example.org>, broken <<, plain@example.net`)
	require.Len(t, got, 3)
	assert.Equal(t, "山田", got[0].Name)
	assert.Equal(t, "yamada@example.com", got[0].Email)
	assert.Equal(t, "Doe, John", got[1].Name)
	assert.Equal(t, "plain@example.net", got[2].Email)
	assert.NotNil(t, ParseAddressList(""))
	assert.Empty(t, ParseAddressList("   "))
}

func TestParseFromAndDecodeHeader(t *testing.T) {
	assert.Equal(t, "a@example.com", ParseFrom("A <a@example.com>").Email)
	assert.Empty(t, ParseFrom("").Email)
	assert.Equal(t, "件名", DecodeHeader(mime.QEncoding.Encode("UTF-8", "件名")))
	assert.Equal(t, "=?bogus?x?zz?=", DecodeHeader("=?bogus?x?zz?="), "デコードできなければ元の値")
}

func TestHeaderValueIsCaseInsensitive(t *testing.T) {
	hs := []*gmailapi.MessagePartHeader{h("message-id", "<a@b>"), nil}
	assert.Equal(t, "<a@b>", HeaderValue(hs, "Message-ID"))
	assert.Empty(t, HeaderValue(hs, "X-None"))
	assert.Equal(t, []string{"<a@b>", "<c@d>"}, SplitFields("<a@b>\r\n <c@d>"))
	assert.NotNil(t, SplitFields(""))
}

func TestDecodeBase64URLAcceptsPadding(t *testing.T) {
	for _, in := range []string{"aGk", "aGk="} {
		b, err := DecodeBase64URL(in)
		require.NoError(t, err)
		assert.Equal(t, "hi", string(b))
	}
	_, err := DecodeBase64URL("!!!")
	require.Error(t, err)
}

func TestDecodeText(t *testing.T) {
	assert.Equal(t, "日本語", DecodeText([]byte("\x1b$BF|K\\8l\x1b(B"), "iso-2022-jp"))
	assert.Equal(t, "日本語", DecodeText([]byte("\x93\xfa\x96\x7b\x8c\xea"), "Shift_JIS"))
	assert.Equal(t, "a\uFFFDb", DecodeText([]byte("a\xffb"), "utf-8"))
	assert.Equal(t, "plain", DecodeText([]byte("plain"), "unknown-charset-x"))
}

func TestWalkClassifiesParts(t *testing.T) {
	root := &gmailapi.MessagePart{MimeType: "multipart/mixed", Parts: []*gmailapi.MessagePart{
		{MimeType: "text/plain", Body: &gmailapi.MessagePartBody{Data: "dA"}},
		{MimeType: "text/plain", Filename: "n.txt", Body: &gmailapi.MessagePartBody{AttachmentId: "a1", Size: 3}},
		{MimeType: "image/png", Headers: []*gmailapi.MessagePartHeader{h("Content-ID", " <x> ")}, Body: &gmailapi.MessagePartBody{AttachmentId: "a2", Size: 9}},
		{MimeType: "application/zip", Body: &gmailapi.MessagePartBody{Size: 1}},
	}}
	p := Walk(root)
	assert.Len(t, p.Text, 1)
	assert.Empty(t, p.HTML)
	require.Len(t, p.Attachments, 1)
	assert.Equal(t, "n.txt", p.Attachments[0].Filename)
	require.Len(t, p.Inlines, 1)
	assert.Equal(t, "x", p.Inlines[0].CID)
	assert.True(t, HasAttachment(root))
	assert.False(t, HasAttachment(&gmailapi.MessagePart{MimeType: "multipart/alternative"}))
	assert.False(t, HasAttachment(nil))
}

func TestNormalizeCID(t *testing.T) {
	cases := map[string]string{
		"<Logo@Example.COM>": "logo@example.com",
		"cid:Logo%40Example": "logo@example",
		" <cid:A%20B> ":      "a b",
		"CID:x":              "x",
		"plain":              "plain",
		"bad%zz":             "bad%zz",
		"<>":                 "",
	}
	for in, want := range cases {
		assert.Equal(t, want, NormalizeCID(in), in)
	}
}

func TestCIDReferences(t *testing.T) {
	refs := CIDReferences(`<img src="cid:Abc"><img src='CID:def%40x'> <div style="background:url(cid:ghi)"></div><img src=cid:jkl>`)
	assert.Equal(t, map[string]bool{"abc": true, "def@x": true, "ghi": true, "jkl": true}, refs)
	assert.False(t, refs["a"], "部分一致しない")
	assert.Empty(t, CIDReferences("<p>no refs</p>"))
}
