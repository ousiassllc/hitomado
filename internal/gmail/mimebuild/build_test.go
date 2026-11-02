package mimebuild

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jhillyerd/enmime/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

func sampleOutgoing() model.OutgoingMessage {
	return model.OutgoingMessage{
		From:       model.Address{Name: "山田 太郎", Email: "taro@example.com"},
		To:         []model.Address{{Name: "花子", Email: "hanako@example.org"}, {Email: "b@example.org"}},
		Cc:         []model.Address{{Email: "cc@example.org"}},
		Bcc:        []model.Address{{Email: "bcc@example.org"}},
		Subject:    "Re: 会議のご案内",
		BodyHTML:   "<p>こんにちは<br>世界</p>",
		InReplyTo:  "<orig@example.org>",
		References: []string{"<r1@example.org>", "<orig@example.org>"},
		Date:       time.Date(2026, 11, 2, 10, 0, 0, 0, time.FixedZone("JST", 9*3600)),
	}
}

func parseMIME(t *testing.T, raw []byte) *enmime.Envelope {
	t.Helper()
	env, err := enmime.ReadEnvelope(bytes.NewReader(raw))
	require.NoError(t, err)
	return env
}

func TestBuildHeadersAndBodies(t *testing.T) {
	raw, err := Build(sampleOutgoing())
	require.NoError(t, err)

	// 日本語の件名・表示名は RFC 2047 でエンコードされる（生のヘッダは ASCII のみ）。
	head, _, _ := strings.Cut(string(raw), "\r\n\r\n")
	assert.NotContains(t, head, "会議")
	assert.Contains(t, head, "=?utf-8?")

	env := parseMIME(t, raw)
	assert.Equal(t, "Re: 会議のご案内", env.GetHeader("Subject"))
	assert.Equal(t, "<orig@example.org>", env.GetHeader("In-Reply-To"))
	assert.Equal(t, "<r1@example.org> <orig@example.org>", env.GetHeader("References"))
	assert.Equal(t, "Mon, 02 Nov 2026 10:00:00 +0900", env.GetHeader("Date"))
	assert.Regexp(t, `^<[0-9A-Z]{26}@example\.com>$`, env.GetHeader("Message-ID"))

	from, err := env.AddressList("From")
	require.NoError(t, err)
	require.Len(t, from, 1)
	assert.Equal(t, "山田 太郎", from[0].Name)
	assert.Equal(t, "taro@example.com", from[0].Address)

	to, err := env.AddressList("To")
	require.NoError(t, err)
	require.Len(t, to, 2)
	assert.Equal(t, "花子", to[0].Name)
	assert.Equal(t, "hanako@example.org", to[0].Address)
	cc, err := env.AddressList("Cc")
	require.NoError(t, err)
	require.Len(t, cc, 1)
	bcc, err := env.AddressList("Bcc")
	require.NoError(t, err)
	require.Len(t, bcc, 1)
	assert.Equal(t, "bcc@example.org", bcc[0].Address)

	assert.Equal(t, "<p>こんにちは<br>世界</p>", strings.TrimSpace(env.HTML))
	assert.Equal(t, "こんにちは\n世界", strings.TrimSpace(strings.ReplaceAll(env.Text, "\r\n", "\n")))
	assert.Empty(t, env.Attachments)
	assert.True(t, strings.HasPrefix(env.Root.ContentType, "multipart/alternative"), env.Root.ContentType)
}

func TestBuildUsesExplicitBodyTextAndDefaultsDate(t *testing.T) {
	m := sampleOutgoing()
	m.BodyText = "明示テキスト"
	m.Date = time.Time{}
	before := time.Now().Add(-time.Minute)
	raw, err := Build(m)
	require.NoError(t, err)
	env := parseMIME(t, raw)
	assert.Equal(t, "明示テキスト", strings.TrimSpace(env.Text))
	d, err := time.Parse(time.RFC1123Z, env.GetHeader("Date"))
	require.NoError(t, err)
	assert.True(t, d.After(before))
}

func TestBuildWithAttachments(t *testing.T) {
	m := sampleOutgoing()
	data := []byte{0, 1, 2, 3, 250, 255}
	m.Attachments = []model.OutgoingAttachment{
		{Filename: "報告書.pdf", MimeType: "application/pdf", Data: data},
		{Filename: "note.txt", Data: []byte("hello")},
	}
	raw, err := Build(m)
	require.NoError(t, err)
	env := parseMIME(t, raw)
	assert.True(t, strings.HasPrefix(env.Root.ContentType, "multipart/mixed"), env.Root.ContentType)
	require.Len(t, env.Attachments, 2)
	assert.Equal(t, "報告書.pdf", env.Attachments[0].FileName)
	assert.Equal(t, "application/pdf", env.Attachments[0].ContentType)
	assert.Equal(t, data, env.Attachments[0].Content)
	assert.Equal(t, "note.txt", env.Attachments[1].FileName)
	assert.Equal(t, "application/octet-stream", env.Attachments[1].ContentType)
	assert.Equal(t, "<p>こんにちは<br>世界</p>", strings.TrimSpace(env.HTML))
}

func TestBuildRejectsHeaderInjection(t *testing.T) {
	cases := map[string]func(m *model.OutgoingMessage){
		"subject":     func(m *model.OutgoingMessage) { m.Subject = "hi\r\nBcc: evil@example.com" },
		"subject LF":  func(m *model.OutgoingMessage) { m.Subject = "hi\nX: y" },
		"from name":   func(m *model.OutgoingMessage) { m.From.Name = "A\r\nBcc: evil@example.com" },
		"from email":  func(m *model.OutgoingMessage) { m.From.Email = "a@example.com\r\nBcc: evil@example.com" },
		"to name":     func(m *model.OutgoingMessage) { m.To[0].Name = "x\r\nBcc: evil@example.com" },
		"to email":    func(m *model.OutgoingMessage) { m.To[0].Email = "x@example.com\nBcc: evil@example.com" },
		"cc email":    func(m *model.OutgoingMessage) { m.Cc[0].Email = "x@example.com\r\n" },
		"bcc name":    func(m *model.OutgoingMessage) { m.Bcc[0].Name = "\r\n" },
		"in-reply-to": func(m *model.OutgoingMessage) { m.InReplyTo = "<a@b>\r\nBcc: evil@example.com" },
		"references":  func(m *model.OutgoingMessage) { m.References = []string{"<a@b>\r\nBcc: evil@example.com"} },
		"attach filename": func(m *model.OutgoingMessage) {
			m.Attachments = []model.OutgoingAttachment{{Filename: "a\r\nX: y", Data: []byte("x")}}
		},
		"attach mime": func(m *model.OutgoingMessage) {
			m.Attachments = []model.OutgoingAttachment{{Filename: "a", MimeType: "text/plain\r\nX: y", Data: []byte("x")}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := sampleOutgoing()
			mutate(&m)
			raw, err := Build(m)
			require.ErrorIs(t, err, ErrInvalid)
			assert.Nil(t, raw)
		})
	}
}

func TestBuildValidation(t *testing.T) {
	m := sampleOutgoing()
	m.To, m.Cc, m.Bcc = nil, nil, nil
	_, err := Build(m)
	require.ErrorIs(t, err, ErrInvalid)

	m = sampleOutgoing()
	m.From.Email = ""
	_, err = Build(m)
	require.ErrorIs(t, err, ErrInvalid)

	m = sampleOutgoing()
	m.To[0].Email = "not an address"
	_, err = Build(m)
	require.ErrorIs(t, err, ErrInvalid)

	m = sampleOutgoing()
	m.To[0].Email = "Name <a@example.org>"
	_, err = Build(m)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestBuildDraftAllowsNoRecipients(t *testing.T) {
	m := sampleOutgoing()
	m.To, m.Cc, m.Bcc = nil, nil, nil
	raw, err := BuildDraft(m)
	require.NoError(t, err)
	env := parseMIME(t, raw)
	assert.Empty(t, env.GetHeader("To"))
	assert.Empty(t, env.GetHeader("Cc"))
	assert.Equal(t, "Re: 会議のご案内", env.GetHeader("Subject"))
	assert.NotContains(t, string(raw), "\r\nTo:")

	// 宛先があるときは Build と同じ。
	raw, err = BuildDraft(sampleOutgoing())
	require.NoError(t, err)
	assert.Equal(t, "hanako@example.org", func() string { a, _ := parseMIME(t, raw).AddressList("To"); return a[0].Address }())

	// ヘッダインジェクションは下書きでも拒否する。
	m.Subject = "x\r\nBcc: a@b.c"
	_, err = BuildDraft(m)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestBuildBccOnly(t *testing.T) {
	for name, build := range map[string]func(model.OutgoingMessage) ([]byte, error){"Build": Build, "BuildDraft": BuildDraft} {
		t.Run(name, func(t *testing.T) {
			m := sampleOutgoing()
			m.To, m.Cc = nil, nil
			raw, err := build(m)
			require.NoError(t, err)
			head, _, _ := strings.Cut(string(raw), "\r\n\r\n")
			assert.NotContains(t, head, "\r\nTo:")
			assert.NotContains(t, head, "taro@example.com\r\nTo")
			env := parseMIME(t, raw)
			assert.Empty(t, env.GetHeader("To"))
			assert.Empty(t, env.GetHeader("Cc"))
			bcc, err := env.AddressList("Bcc")
			require.NoError(t, err)
			require.Len(t, bcc, 1)
			assert.Equal(t, "bcc@example.org", bcc[0].Address)
			assert.Equal(t, 1, strings.Count(head, "From:"))
		})
	}
}

func TestBuildCcOnlyKeepsCcWithoutTo(t *testing.T) {
	m := sampleOutgoing()
	m.To = nil
	raw, err := Build(m)
	require.NoError(t, err)
	env := parseMIME(t, raw)
	assert.Empty(t, env.GetHeader("To"))
	assert.Equal(t, "cc@example.org", func() string { a, _ := env.AddressList("Cc"); return a[0].Address }())
}
