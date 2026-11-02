package gmailtests

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/gmail/gmailtest"
)

type rawBody struct {
	Raw      string `json:"raw"`
	ThreadID string `json:"threadId"`
}

// draftBody は drafts.create／update のリクエスト本文。
type draftBody struct {
	Message rawBody `json:"message"`
}

func TestSendUsesMediaUpload(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("POST "+sendUpload, 200, map[string]any{"id": "sent-1", "threadId": "t9"})

	mime := []byte("From: a@example.com\r\n\r\n日本語 body\xff")
	got, err := e.c.Send(context.Background(), testAccount, mime, "t9")
	require.NoError(t, err)
	assert.Equal(t, gmail.SentMessage{MessageID: "sent-1", ThreadID: "t9"}, got)

	reqs := e.f.Requests("POST", sendUpload)
	require.Len(t, reqs, 1)
	assert.Equal(t, "Bearer at-1", reqs[0].Header.Get("Authorization"))
	up := gmailtest.ParseUpload(t, reqs[0])
	assert.Equal(t, "multipart", up.UploadType)
	assert.Equal(t, "message/rfc822", up.MediaType)
	assert.Equal(t, mime, up.Media)
	var meta rawBody
	require.NoError(t, json.Unmarshal(up.Metadata, &meta))
	assert.Equal(t, "t9", meta.ThreadID)
	assert.Empty(t, meta.Raw, "raw は JSON に埋め込まない")
	assert.Empty(t, e.f.Requests("POST", apiBase+"/messages/send"), "JSON 経路は使わない")
}

func TestSendLargeMessageUploadsWholeBody(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("POST "+sendUpload, 200, map[string]any{"id": "s", "threadId": "t"})
	big := bytes.Repeat([]byte("0123456789abcdef"), 2<<20) // 32MB。既定のチャンク上限（16MB）を超える
	_, err := e.c.Send(context.Background(), testAccount, big, "")
	require.NoError(t, err)
	reqs := e.f.Requests("POST", sendUpload)
	require.Len(t, reqs, 1, "再開可能アップロードに切り替えず 1 リクエストで送る")
	assert.Equal(t, big, gmailtest.ParseUpload(t, reqs[0]).Media)
}

func TestSendRetryResendsFullBody(t *testing.T) {
	e := newTestEnv(t)
	var calls atomic.Int32
	e.f.Handle("POST "+sendUpload, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writeJSON(w, 429, apiError(429, "rateLimitExceeded", "slow"))
			return
		}
		writeJSON(w, 200, map[string]any{"id": "m", "threadId": "t"})
	})
	_, err := e.c.Send(context.Background(), testAccount, []byte("payload"), "")
	require.NoError(t, err)
	reqs := e.f.Requests("POST", sendUpload)
	require.Len(t, reqs, 2)
	assert.Equal(t, []byte("payload"), gmailtest.ParseUpload(t, reqs[1]).Media)
}

func TestSendNewThreadOmitsThreadID(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("POST "+sendUpload, 200, map[string]any{"id": "s", "threadId": "t"})
	_, err := e.c.Send(context.Background(), testAccount, []byte("x"), "")
	require.NoError(t, err)
	up := gmailtest.ParseUpload(t, e.f.Requests("POST", sendUpload)[0])
	var meta map[string]any
	require.NoError(t, json.Unmarshal(up.Metadata, &meta))
	assert.NotContains(t, meta, "threadId")
	assert.Equal(t, []byte("x"), up.Media)
}

func TestCreateAndUpdateDraft(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("POST "+draftsUpload, 200, map[string]any{"id": "d1", "message": map[string]any{"id": "m", "threadId": "t1"}})
	e.f.HandleJSON("PUT "+draftsUpload+"/d1", 200, map[string]any{"id": "d1"})

	id, err := e.c.CreateDraft(context.Background(), testAccount, []byte("raw-1"), "t1")
	require.NoError(t, err)
	assert.Equal(t, "d1", id)
	created := gmailtest.ParseUpload(t, e.f.Requests("POST", draftsUpload)[0])
	assert.Equal(t, "message/rfc822", created.MediaType)
	assert.Equal(t, []byte("raw-1"), created.Media)
	var cm draftBody
	require.NoError(t, json.Unmarshal(created.Metadata, &cm))
	assert.Equal(t, "t1", cm.Message.ThreadID)
	assert.Empty(t, cm.Message.Raw)

	require.NoError(t, e.c.UpdateDraft(context.Background(), testAccount, "d1", []byte("raw-2")))
	updated := gmailtest.ParseUpload(t, e.f.Requests("PUT", draftsUpload+"/d1")[0])
	assert.Equal(t, "message/rfc822", updated.MediaType)
	assert.Equal(t, []byte("raw-2"), updated.Media)
}

func TestUpdateDraftNotFound(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("PUT "+draftsUpload+"/nope", 404, apiError(404, "notFound", "nf"))
	err := e.c.UpdateDraft(context.Background(), testAccount, "nope", []byte("x"))
	require.ErrorIs(t, err, gmail.ErrNotFound)
}

func TestDeleteDraft(t *testing.T) {
	e := newTestEnv(t)
	e.f.Handle("DELETE "+apiBase+"/drafts/d1", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	require.NoError(t, e.c.DeleteDraft(context.Background(), testAccount, "d1"))
	assert.Len(t, e.f.Requests("DELETE", apiBase+"/drafts/d1"), 1)
}

func TestGetDraft(t *testing.T) {
	e := newTestEnv(t)
	html := base64.URLEncoding.EncodeToString([]byte("<p>下書き</p>"))
	e.f.HandleJSON("GET "+apiBase+"/drafts/d1", 200, map[string]any{
		"id": "d1",
		"message": map[string]any{
			"id": "m1", "threadId": "t1", "labelIds": []string{"DRAFT"}, "internalDate": "1700000000000",
			"payload": map[string]any{
				"mimeType": "multipart/mixed",
				"headers":  []map[string]any{hdr("Subject", "下書き"), hdr("To", "a@example.com"), hdr("Bcc", "=?UTF-8?B?5bGx55Sw?= <Hidden@Example.com>, b2@example.com"), hdr("In-Reply-To", "<x@y>")},
				"parts": []map[string]any{
					{"mimeType": "text/html", "body": map[string]any{"data": html}},
					{"mimeType": "application/pdf", "filename": "a.pdf", "body": map[string]any{"attachmentId": "att1", "size": 42}},
				},
			},
		},
	})
	got, err := e.c.GetDraft(context.Background(), testAccount, "d1")
	require.NoError(t, err)
	assert.Equal(t, "d1", got.ID)
	assert.Equal(t, "t1", got.ThreadID)
	assert.Equal(t, "<p>下書き</p>", got.Message.BodyHTML)
	assert.Equal(t, "下書き", got.Message.Subject)
	require.Len(t, got.Bcc, 2)
	assert.Equal(t, "山田", got.Bcc[0].Name)
	assert.Equal(t, "hidden@example.com", got.Bcc[0].Email)
	assert.Equal(t, "b2@example.com", got.Bcc[1].Email)
	require.Len(t, got.Attachments, 1)
	assert.Equal(t, "att1", got.Attachments[0].ID)
	assert.Equal(t, "m1", got.Message.ID)
	a := got.Attachments[0]
	assert.Equal(t, []any{"a.pdf", "application/pdf", int64(42)}, []any{a.Filename, a.MimeType, a.Size})
	assert.Equal(t, "full", e.f.Requests("GET", apiBase+"/drafts/d1")[0].Query.Get("format"))
}

func TestFindDraftForThreadWalksAllPages(t *testing.T) {
	e := newTestEnv(t)
	e.f.Handle("GET "+apiBase+"/drafts", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("pageToken") {
		case "":
			writeJSON(w, 200, map[string]any{"nextPageToken": "p2", "drafts": []map[string]any{{"id": "d1", "message": map[string]any{"id": "m1", "threadId": "tA"}}}})
		case "p2":
			writeJSON(w, 200, map[string]any{"drafts": []map[string]any{{"id": "d2", "message": map[string]any{"id": "m2", "threadId": "tB"}}}})
		default:
			t.Errorf("想定外の pageToken")
		}
	})

	id, ok, err := e.c.FindDraftForThread(context.Background(), testAccount, "tB")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "d2", id)

	id, ok, err = e.c.FindDraftForThread(context.Background(), testAccount, "none")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Empty(t, id)
}

func TestGetDraftWithoutPayloadHasEmptyBcc(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/drafts/d2", 200, map[string]any{"id": "d2", "message": map[string]any{"id": "m", "threadId": "t"}})
	got, err := e.c.GetDraft(context.Background(), testAccount, "d2")
	require.NoError(t, err)
	assert.NotNil(t, got.Bcc)
	assert.Empty(t, got.Bcc)
}
