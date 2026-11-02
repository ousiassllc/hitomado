package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rawBody struct {
	Raw      string `json:"raw"`
	ThreadID string `json:"threadId"`
}

// draftBody は drafts.create／update のリクエスト本文。
type draftBody struct {
	Message rawBody `json:"message"`
}

func TestSend(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("POST "+apiBase+"/messages/send", 200, map[string]any{"id": "sent-1", "threadId": "t9"})

	mime := []byte("From: a@example.com\r\n\r\n日本語 body\xff")
	got, err := e.c.Send(context.Background(), testAccount, mime, "t9")
	require.NoError(t, err)
	assert.Equal(t, SentMessage{MessageID: "sent-1", ThreadID: "t9"}, got)

	var body rawBody
	require.NoError(t, json.Unmarshal(e.f.Requests("POST", apiBase+"/messages/send")[0].Body, &body))
	assert.Equal(t, "t9", body.ThreadID)
	decoded, err := base64.URLEncoding.DecodeString(body.Raw)
	require.NoError(t, err)
	assert.Equal(t, mime, decoded)
}

func TestSendNewThreadOmitsThreadID(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("POST "+apiBase+"/messages/send", 200, map[string]any{"id": "s", "threadId": "t"})
	_, err := e.c.Send(context.Background(), testAccount, []byte("x"), "")
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(e.f.Requests("POST", apiBase+"/messages/send")[0].Body, &body))
	assert.NotContains(t, body, "threadId")
}

func TestCreateAndUpdateDraft(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("POST "+apiBase+"/drafts", 200, map[string]any{"id": "d1", "message": map[string]any{"id": "m", "threadId": "t1"}})
	e.f.HandleJSON("PUT "+apiBase+"/drafts/d1", 200, map[string]any{"id": "d1"})

	id, err := e.c.CreateDraft(context.Background(), testAccount, []byte("raw-1"), "t1")
	require.NoError(t, err)
	assert.Equal(t, "d1", id)
	var created draftBody
	require.NoError(t, json.Unmarshal(e.f.Requests("POST", apiBase+"/drafts")[0].Body, &created))
	assert.Equal(t, "t1", created.Message.ThreadID)
	assert.Equal(t, base64.URLEncoding.EncodeToString([]byte("raw-1")), created.Message.Raw)

	require.NoError(t, e.c.UpdateDraft(context.Background(), testAccount, "d1", []byte("raw-2")))
	var updated draftBody
	require.NoError(t, json.Unmarshal(e.f.Requests("PUT", apiBase+"/drafts/d1")[0].Body, &updated))
	assert.Equal(t, base64.URLEncoding.EncodeToString([]byte("raw-2")), updated.Message.Raw)
}

func TestUpdateDraftNotFound(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("PUT "+apiBase+"/drafts/nope", 404, apiError(404, "notFound", "nf"))
	err := e.c.UpdateDraft(context.Background(), testAccount, "nope", []byte("x"))
	require.ErrorIs(t, err, ErrNotFound)
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
				"headers":  []map[string]any{hdr("Subject", "下書き"), hdr("To", "a@example.com"), hdr("In-Reply-To", "<x@y>")},
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
	require.Len(t, got.Attachments, 1)
	assert.Equal(t, "att1", got.Attachments[0].ID)
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
