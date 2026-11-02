package gmail

import (
	"context"
	"encoding/base64"
	"mime"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail/mimeparse"
)

func b64(s string) string { return base64.URLEncoding.EncodeToString([]byte(s)) }

func fullMessage(id string, payload map[string]any) map[string]any {
	return map[string]any{"id": id, "threadId": "t1", "internalDate": "1700000000000", "labelIds": []string{"INBOX"}, "payload": payload}
}

func TestGetThreadFullAlternativeWithInlineAndAttachment(t *testing.T) {
	e := newTestEnv(t)
	png := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	big := make([]byte, mimeparse.MaxInlineImageBytes+1)
	e.f.HandleJSON("GET "+apiBase+"/threads/t1", 200, map[string]any{
		"id": "t1", "historyId": "77",
		"messages": []map[string]any{
			fullMessage("m1", map[string]any{
				"mimeType": "multipart/mixed",
				"headers": []map[string]any{
					hdr("From", "Alice <alice@example.org>"), hdr("To", "me@example.com"), hdr("Subject", mime.BEncoding.Encode("UTF-8", "案件")),
					hdr("Message-ID", " <abc@example.org> "), hdr("References", "<r1@example.org>\r\n <r2@example.org>"),
					hdr("Reply-To", "Support <support@example.org>"),
				},
				"parts": []map[string]any{
					{"mimeType": "multipart/related", "parts": []map[string]any{
						{"mimeType": "multipart/alternative", "parts": []map[string]any{
							{"mimeType": "text/plain", "headers": []map[string]any{hdr("Content-Type", `text/plain; charset="UTF-8"`)}, "body": map[string]any{"data": b64("plain 本文")}},
							{"mimeType": "text/html", "body": map[string]any{"data": b64(`<p>hi <img src="cid:logo123"><img src="cid:huge"><img src="cid:gone"></p>`)}},
						}},
						{"mimeType": "image/png", "filename": "logo.png", "headers": []map[string]any{hdr("Content-ID", "<logo123>"), hdr("Content-Disposition", `inline; filename="logo.png"`)},
							"body": map[string]any{"attachmentId": "att-logo", "size": len(png)}},
						{"mimeType": "image/png", "headers": []map[string]any{hdr("Content-ID", "<huge>")},
							"body": map[string]any{"attachmentId": "att-huge", "size": len(big)}},
						{"mimeType": "image/gif", "headers": []map[string]any{hdr("Content-ID", "<gone>")},
							"body": map[string]any{"attachmentId": "att-gone", "size": 10}},
					}},
					{"mimeType": "application/pdf", "filename": "報告書.pdf", "body": map[string]any{"attachmentId": "att-pdf", "size": 2048}},
				},
			}),
			fullMessage("m2", map[string]any{
				"mimeType": "text/plain",
				"headers":  []map[string]any{hdr("From", "me@example.com"), hdr("Subject", "Re: 案件")},
				"body":     map[string]any{"data": b64("返信です")},
			}),
		},
	})
	e.f.HandleJSON("GET "+apiBase+"/messages/m1/attachments/att-logo", 200, map[string]any{"data": base64.RawURLEncoding.EncodeToString(png), "size": len(png)})
	e.f.HandleJSON("GET "+apiBase+"/messages/m1/attachments/att-gone", 404, apiError(404, "notFound", "nf"))

	got, err := e.c.GetThreadFull(context.Background(), testAccount, "t1")
	require.NoError(t, err)
	assert.Equal(t, "77", got.HistoryID)
	require.Len(t, got.Messages, 2)

	m := got.Messages[0]
	assert.Equal(t, "案件", m.Subject)
	assert.Equal(t, "alice@example.org", m.From.Email)
	assert.Equal(t, "<abc@example.org>", m.MessageID)
	assert.Equal(t, []string{"<r1@example.org>", "<r2@example.org>"}, m.References)
	require.Len(t, m.ReplyTo, 1)
	assert.Equal(t, "support@example.org", m.ReplyTo[0].Email)
	assert.Equal(t, "plain 本文", m.BodyText)
	assert.Contains(t, m.BodyHTML, `cid:logo123`)
	assert.True(t, m.HasAttachment)

	require.Len(t, m.Attachments, 1)
	assert.Equal(t, "att-pdf", m.Attachments[0].ID)
	assert.Equal(t, "報告書.pdf", m.Attachments[0].Filename)
	assert.Equal(t, "application/pdf", m.Attachments[0].MimeType)
	assert.EqualValues(t, 2048, m.Attachments[0].Size)

	require.Contains(t, m.InlineImages, "logo123")
	assert.Equal(t, InlinePart{MimeType: "image/png", Data: png}, m.InlineImages["logo123"])
	assert.NotContains(t, m.InlineImages, "huge", "5MB 超はスキップ")
	assert.NotContains(t, m.InlineImages, "gone", "取得不能な画像はスキップ")
	assert.Empty(t, e.f.Requests("GET", apiBase+"/messages/m1/attachments/att-huge"), "5MB 超は取得もしない")

	m2 := got.Messages[1]
	assert.Equal(t, "返信です", m2.BodyText)
	assert.Empty(t, m2.BodyHTML)
	assert.NotNil(t, m2.Attachments)
	assert.NotNil(t, m2.InlineImages)
	assert.False(t, m2.HasAttachment)

	assert.Equal(t, "full", e.f.Requests("GET", apiBase+"/threads/t1")[0].Query.Get("format"))
}

func TestGetThreadFullFetchesLargeBodyByAttachmentID(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/threads/t1", 200, map[string]any{
		"id": "t1",
		"messages": []map[string]any{fullMessage("m1", map[string]any{
			"mimeType": "multipart/alternative",
			"parts": []map[string]any{
				{"mimeType": "text/html", "body": map[string]any{"attachmentId": "body-att", "size": 9999999}},
			},
		})},
	})
	e.f.HandleJSON("GET "+apiBase+"/messages/m1/attachments/body-att", 200, map[string]any{"data": b64("<p>大きい本文</p>")})
	got, err := e.c.GetThreadFull(context.Background(), testAccount, "t1")
	require.NoError(t, err)
	assert.Equal(t, "<p>大きい本文</p>", got.Messages[0].BodyHTML)
	assert.Empty(t, got.Messages[0].Attachments)
}

func TestGetThreadFullConvertsLegacyCharset(t *testing.T) {
	e := newTestEnv(t)
	// "日本語" の ISO-2022-JP。
	iso := "\x1b$BF|K\\8l\x1b(B"
	e.f.HandleJSON("GET "+apiBase+"/threads/t1", 200, map[string]any{
		"id": "t1",
		"messages": []map[string]any{fullMessage("m1", map[string]any{
			"mimeType": "text/plain",
			"headers":  []map[string]any{hdr("Content-Type", "text/plain; charset=ISO-2022-JP")},
			"body":     map[string]any{"data": b64(iso)},
		})},
	})
	got, err := e.c.GetThreadFull(context.Background(), testAccount, "t1")
	require.NoError(t, err)
	assert.Equal(t, "日本語", got.Messages[0].BodyText)
}

func TestGetThreadFullKeepsDraftsAndAttachmentDispositionImages(t *testing.T) {
	e := newTestEnv(t)
	img := []byte("imgdata")
	msg := fullMessage("m1", map[string]any{
		"mimeType": "multipart/mixed",
		"parts": []map[string]any{
			{"mimeType": "text/html", "body": map[string]any{"data": b64("<p>no refs</p>")}},
			{"mimeType": "image/jpeg", "filename": "photo.jpg", "headers": []map[string]any{hdr("Content-Disposition", "attachment; filename=photo.jpg")},
				"body": map[string]any{"attachmentId": "att-photo", "size": len(img)}},
		},
	})
	msg["labelIds"] = []string{"DRAFT"}
	e.f.HandleJSON("GET "+apiBase+"/threads/t1", 200, map[string]any{"id": "t1", "messages": []map[string]any{msg}})
	got, err := e.c.GetThreadFull(context.Background(), testAccount, "t1")
	require.NoError(t, err)
	assert.Equal(t, []string{"DRAFT"}, got.Messages[0].LabelIDs)
	require.Len(t, got.Messages[0].Attachments, 1)
	assert.Equal(t, "photo.jpg", got.Messages[0].Attachments[0].Filename)
}

func TestGetThreadFullPropagatesNotFound(t *testing.T) {
	e := newTestEnv(t)
	e.f.Handle("GET "+apiBase+"/threads/zzz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 404, apiError(404, "notFound", "nf"))
	})
	_, err := e.c.GetThreadFull(context.Background(), testAccount, "zzz")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestGetThreadFullCIDMatchingIsExactAndNormalized(t *testing.T) {
	e := newTestEnv(t)
	img := func(cid, att string) map[string]any {
		return map[string]any{"mimeType": "image/png", "headers": []map[string]any{hdr("Content-ID", cid)}, "body": map[string]any{"attachmentId": att, "size": 3}}
	}
	e.f.HandleJSON("GET "+apiBase+"/threads/t1", 200, map[string]any{
		"id": "t1",
		"messages": []map[string]any{fullMessage("m1", map[string]any{
			"mimeType": "multipart/related",
			"parts": []map[string]any{
				{"mimeType": "text/html", "body": map[string]any{"data": b64(`<img src="cid:UPPER"><img src="cid:at%40host"><img src="cid:abc">`)}},
				img("<upper>", "att-upper"),
				img("<at@host>", "att-at"),
				img("<a>", "att-a"), // cid:abc の前方一致であってはならない
			},
		})},
	})
	for _, a := range []string{"att-upper", "att-at"} {
		e.f.HandleJSON("GET "+apiBase+"/messages/m1/attachments/"+a, 200, map[string]any{"data": b64(a)})
	}
	got, err := e.c.GetThreadFull(context.Background(), testAccount, "t1")
	require.NoError(t, err)
	m := got.Messages[0]
	assert.Equal(t, "att-upper", string(m.InlineImages["upper"].Data))
	assert.Equal(t, "att-at", string(m.InlineImages["at@host"].Data))
	assert.NotContains(t, m.InlineImages, "a")
	assert.Empty(t, e.f.Requests("GET", apiBase+"/messages/m1/attachments/att-a"))
	require.Len(t, m.Attachments, 1, "参照されない画像は添付一覧に回る")
	assert.Equal(t, "att-a", m.Attachments[0].ID)
}
