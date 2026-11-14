package gmailtests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

func TestListThreads(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/threads", 200, map[string]any{
		"threads":       []map[string]any{{"id": "t1", "historyId": "99", "snippet": "hello"}, {"id": "t2", "historyId": "100"}},
		"nextPageToken": "next",
	})

	got, err := e.c.ListThreads(context.Background(), testAccount, gmail.ListQuery{Q: "from:a", LabelIDs: []string{"INBOX", "UNREAD"}, PageToken: "p1", MaxResults: 50})
	require.NoError(t, err)
	assert.Equal(t, gmail.ThreadList{
		Threads:       []gmail.ThreadStub{{ID: "t1", HistoryID: "99", Snippet: "hello"}, {ID: "t2", HistoryID: "100"}},
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
	got, err := e.c.ListThreads(context.Background(), testAccount, gmail.ListQuery{})
	require.NoError(t, err)
	assert.NotNil(t, got.Threads)
	assert.Empty(t, got.Threads)
	assert.Empty(t, e.f.Requests("GET", apiBase+"/threads")[0].Query.Get("q"))
}

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
