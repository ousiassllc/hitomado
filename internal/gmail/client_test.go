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

const apiBase = "/gmail/v1/users/me"

func TestProfile(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 200, map[string]any{"emailAddress": "Me@Example.COM", "historyId": "12345"})

	p, err := e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	assert.Equal(t, Profile{Email: "me@example.com", HistoryID: "12345"}, p)

	reqs := e.f.Requests("GET", apiBase+"/profile")
	require.Len(t, reqs, 1)
	assert.Equal(t, "Bearer at-1", reqs[0].Header.Get("Authorization"))
}

func TestTokenIsCachedAcrossCalls(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 200, map[string]any{"emailAddress": "a@example.com", "historyId": "1"})
	for range 3 {
		_, err := e.c.Profile(context.Background(), testAccount)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, e.f.TokenRequests())
}

func TestAccountLimiterIsPerAccountAt250(t *testing.T) {
	e := newTestEnv(t)
	a, err := e.c.account(context.Background(), testAccount)
	require.NoError(t, err)
	assert.EqualValues(t, 250, a.limiter.Burst())
	assert.EqualValues(t, 250, a.limiter.Limit())
}

func TestListThreads(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/threads", 200, map[string]any{
		"threads":       []map[string]any{{"id": "t1", "historyId": "99", "snippet": "hello"}, {"id": "t2", "historyId": "100"}},
		"nextPageToken": "next",
	})

	got, err := e.c.ListThreads(context.Background(), testAccount, ListQuery{Q: "from:a", LabelIDs: []string{"INBOX", "UNREAD"}, PageToken: "p1", MaxResults: 50})
	require.NoError(t, err)
	assert.Equal(t, ThreadList{
		Threads:       []ThreadStub{{ID: "t1", HistoryID: "99", Snippet: "hello"}, {ID: "t2", HistoryID: "100"}},
		NextPageToken: "next",
	}, got)

	q := e.f.Requests("GET", apiBase+"/threads")[0].Query
	assert.Equal(t, "from:a", q.Get("q"))
	assert.Equal(t, []string{"INBOX", "UNREAD"}, q["labelIds"])
	assert.Equal(t, "p1", q.Get("pageToken"))
	assert.Equal(t, "50", q.Get("maxResults"))
}

func TestListThreadsEmptyIsNonNil(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/threads", 200, map[string]any{})
	got, err := e.c.ListThreads(context.Background(), testAccount, ListQuery{})
	require.NoError(t, err)
	assert.NotNil(t, got.Threads)
	assert.Empty(t, got.Threads)
	assert.Empty(t, e.f.Requests("GET", apiBase+"/threads")[0].Query.Get("q"))
}

func hdr(name, value string) map[string]any { return map[string]any{"name": name, "value": value} }

func TestGetThreadMetadata(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/threads/t1", 200, map[string]any{
		"id": "t1", "historyId": "500",
		"messages": []map[string]any{{
			"id": "m1", "threadId": "t1", "internalDate": "1700000000000", "labelIds": []string{"INBOX", "UNREAD"}, "snippet": "snip",
			"payload": map[string]any{"mimeType": "multipart/mixed", "headers": []map[string]any{
				hdr("From", "=?UTF-8?B?5bGx55Sw5aSq6YOO?= <Taro@Example.com>"),
				hdr("To", "a@example.com, \"B, Bob\" <b@example.com>"),
				hdr("Cc", "=?ISO-2022-JP?B?GyRCRnxLXDhsGyhC?= <c@example.com>"),
				hdr("Subject", "=?UTF-8?B?44GT44KT44Gr44Gh44Gv?="),
				hdr("Date", "Tue, 14 Nov 2023 22:13:20 +0000"),
			}},
		}, {
			"id": "m2", "threadId": "t1", "internalDate": "1700000100000",
			"payload": map[string]any{"mimeType": "text/plain", "headers": []map[string]any{hdr("From", "x@example.com"), hdr("Subject", "Re: plain")}},
		}},
	})

	got, err := e.c.GetThreadMetadata(context.Background(), testAccount, "t1")
	require.NoError(t, err)
	assert.Equal(t, "t1", got.ID)
	assert.Equal(t, "500", got.HistoryID)
	require.Len(t, got.Messages, 2)

	m := got.Messages[0]
	assert.Equal(t, "m1", m.ID)
	assert.Equal(t, int64(1700000000000), m.InternalDate.UnixMilli())
	assert.Equal(t, []string{"INBOX", "UNREAD"}, m.LabelIDs)
	assert.Equal(t, "山田太郎", m.From.Name)
	assert.Equal(t, "taro@example.com", m.From.Email)
	require.Len(t, m.To, 2)
	assert.Equal(t, "B, Bob", m.To[1].Name)
	require.Len(t, m.Cc, 1)
	assert.Equal(t, "日本語", m.Cc[0].Name)
	assert.Equal(t, "こんにちは", m.Subject)
	assert.True(t, m.HasAttachment)

	m2 := got.Messages[1]
	assert.False(t, m2.HasAttachment)
	assert.NotNil(t, m2.To)
	assert.Empty(t, m2.Cc)

	q := e.f.Requests("GET", apiBase+"/threads/t1")[0].Query
	assert.Equal(t, "metadata", q.Get("format"))
	assert.Equal(t, []string{"From", "To", "Cc", "Subject", "Date"}, q["metadataHeaders"])
}

func TestGetThreadMetadataNotFound(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/threads/gone", 404, apiError(404, "notFound", "Requested entity was not found."))
	_, err := e.c.GetThreadMetadata(context.Background(), testAccount, "gone")
	require.ErrorIs(t, err, ErrNotFound)
	assert.NotErrorIs(t, err, ErrGeneric)
	assert.Empty(t, e.sleeper.recorded())
}

func TestListHistoryMergesPages(t *testing.T) {
	e := newTestEnv(t)
	e.f.Handle("GET "+apiBase+"/history", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			writeJSON(w, 200, map[string]any{
				"historyId": "900", "nextPageToken": "pg2",
				"history": []map[string]any{
					{"id": "801", "messagesAdded": []map[string]any{{"message": map[string]any{"id": "m1", "threadId": "tA", "labelIds": []string{"INBOX", "UNREAD"}}}}},
					{"id": "802", "labelsAdded": []map[string]any{{"message": map[string]any{"id": "m1", "threadId": "tA"}, "labelIds": []string{"STARRED"}}}},
				},
			})
			return
		}
		writeJSON(w, 200, map[string]any{
			"historyId": "950",
			"history": []map[string]any{
				{"id": "810", "messagesDeleted": []map[string]any{{"message": map[string]any{"id": "m2", "threadId": "tB"}}}},
				{"id": "811", "labelsRemoved": []map[string]any{{"message": map[string]any{"id": "m3", "threadId": "tC"}, "labelIds": []string{"UNREAD"}}}},
				{"id": "812", "messagesAdded": []map[string]any{{"message": map[string]any{"id": "m4", "threadId": "tA", "labelIds": []string{"SENT"}}}}},
			},
		})
	})

	got, err := e.c.ListHistory(context.Background(), testAccount, "800")
	require.NoError(t, err)
	assert.Equal(t, "950", got.LatestHistoryID)
	assert.Equal(t, []string{"tA", "tB", "tC"}, got.ChangedThreadIDs)
	assert.Equal(t, []HistoryAdded{
		{MessageID: "m1", ThreadID: "tA", LabelIDs: []string{"INBOX", "UNREAD"}},
		{MessageID: "m4", ThreadID: "tA", LabelIDs: []string{"SENT"}},
	}, got.Added)

	reqs := e.f.Requests("GET", apiBase+"/history")
	require.Len(t, reqs, 2)
	assert.Equal(t, "800", reqs[0].Query.Get("startHistoryId"))
	assert.Equal(t, "pg2", reqs[1].Query.Get("pageToken"))
	assert.ElementsMatch(t, []string{"messageAdded", "messageDeleted", "labelAdded", "labelRemoved"}, reqs[0].Query["historyTypes"])
}

func TestListHistoryNoChangesKeepsStartID(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/history", 200, map[string]any{})
	got, err := e.c.ListHistory(context.Background(), testAccount, "800")
	require.NoError(t, err)
	assert.Equal(t, "800", got.LatestHistoryID)
	assert.NotNil(t, got.ChangedThreadIDs)
	assert.NotNil(t, got.Added)
}

func TestListHistoryExpired(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/history", 404, apiError(404, "notFound", "Requested entity was not found."))
	_, err := e.c.ListHistory(context.Background(), testAccount, "1")
	require.ErrorIs(t, err, ErrHistoryExpired)
}

func TestModifyAndTrashThread(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("POST "+apiBase+"/threads/t1/modify", 200, map[string]any{"id": "t1"})
	e.f.HandleJSON("POST "+apiBase+"/threads/t1/trash", 200, map[string]any{"id": "t1"})

	require.NoError(t, e.c.ModifyThread(context.Background(), testAccount, "t1", []string{"UNREAD"}, []string{"INBOX"}))
	var body struct {
		Add    []string `json:"addLabelIds"`
		Remove []string `json:"removeLabelIds"`
	}
	require.NoError(t, json.Unmarshal(e.f.Requests("POST", apiBase+"/threads/t1/modify")[0].Body, &body))
	assert.Equal(t, []string{"UNREAD"}, body.Add)
	assert.Equal(t, []string{"INBOX"}, body.Remove)

	require.NoError(t, e.c.TrashThread(context.Background(), testAccount, "t1"))
	assert.Len(t, e.f.Requests("POST", apiBase+"/threads/t1/trash"), 1)

	e.f.HandleJSON("POST "+apiBase+"/threads/t1/untrash", 200, map[string]any{"id": "t1"})
	require.NoError(t, e.c.UntrashThread(context.Background(), testAccount, "t1"))
	assert.Len(t, e.f.Requests("POST", apiBase+"/threads/t1/untrash"), 1)
}

func TestGetAttachment(t *testing.T) {
	e := newTestEnv(t)
	payload := []byte{0, 1, 2, 250, 251, 252, 255}
	e.f.HandleJSON("GET "+apiBase+"/messages/m1/attachments/a1", 200, map[string]any{
		"size": len(payload), "data": base64.RawURLEncoding.EncodeToString(payload),
	})
	got, err := e.c.GetAttachment(context.Background(), testAccount, "m1", "a1")
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

func TestGetAttachmentGone(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/messages/m1/attachments/a1", 404, apiError(404, "notFound", "nf"))
	_, err := e.c.GetAttachment(context.Background(), testAccount, "m1", "a1")
	require.ErrorIs(t, err, ErrAttachmentGone)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestListLabelsFetchesColorForUserLabels(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/labels", 200, map[string]any{"labels": []map[string]any{
		{"id": "INBOX", "name": "INBOX", "type": "system"},
		{"id": "Label_1", "name": "仕事", "type": "user"},
		{"id": "Label_2", "name": "削除済み", "type": "user"},
	}})
	e.f.HandleJSON("GET "+apiBase+"/labels/Label_1", 200, map[string]any{
		"id": "Label_1", "name": "仕事", "type": "user", "color": map[string]any{"backgroundColor": "#16a766", "textColor": "#ffffff"},
	})
	e.f.HandleJSON("GET "+apiBase+"/labels/Label_2", 404, apiError(404, "notFound", "nf"))

	got, err := e.c.ListLabels(context.Background(), testAccount)
	require.NoError(t, err)
	assert.Equal(t, []Label{
		{ID: "INBOX", Name: "INBOX", Type: "system"},
		{ID: "Label_1", Name: "仕事", Type: "user", ColorBg: "#16a766", ColorFg: "#ffffff"},
		{ID: "Label_2", Name: "削除済み", Type: "user"},
	}, got)
}

func TestGetSignature(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/settings/sendAs/me@example.com", 200, map[string]any{
		"sendAsEmail": "me@example.com", "signature": `<div dir="ltr">山田</div>`,
	})
	got, err := e.c.GetSignature(context.Background(), testAccount, "me@example.com")
	require.NoError(t, err)
	assert.Equal(t, `<div dir="ltr">山田</div>`, got)
}

func TestGetSignatureEmpty(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/settings/sendAs/me@example.com", 200, map[string]any{"sendAsEmail": "me@example.com"})
	got, err := e.c.GetSignature(context.Background(), testAccount, "me@example.com")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGetSignatureNotFound(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/settings/sendAs/me@example.com", 404, apiError(404, "notFound", "nf"))
	_, err := e.c.GetSignature(context.Background(), testAccount, "me@example.com")
	require.ErrorIs(t, err, ErrNotFound)
}
