package gmailtests

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

// metadataDraft は drafts.get format=metadata の応答。
func metadataDraft(id, threadID, subject, snippet string, internalDate int64, headers ...map[string]any) map[string]any {
	h := append([]map[string]any{hdr("Subject", subject), hdr("To", "=?UTF-8?B?5bGx55Sw?= <Yamada@Example.com>, b@example.com")}, headers...)
	return map[string]any{
		"id": id,
		"message": map[string]any{
			"id": "m-" + id, "threadId": threadID, "labelIds": []string{"DRAFT"}, "snippet": snippet,
			"internalDate": strconv.FormatInt(internalDate, 10),
			"payload":      map[string]any{"mimeType": "multipart/alternative", "headers": h},
		},
	}
}

// 下書きの ID を drafts.list で集め、各下書きを format=metadata で読む。宛先は To、本文の先頭は snippet（HTML のエスケープを戻す）、
// In-Reply-To があれば返信の下書き。
func TestListDraftsReadsSummaries(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/drafts", 200, map[string]any{"drafts": []map[string]any{
		{"id": "d1", "message": map[string]any{"id": "m-d1", "threadId": "t1"}},
		{"id": "d2", "message": map[string]any{"id": "m-d2", "threadId": "t2"}},
	}})
	e.f.HandleJSON("GET "+apiBase+"/drafts/d1", 200, metadataDraft("d1", "t1", "新規の件名", "It&#39;s a &quot;draft&quot; &amp; more", 1700000001000))
	e.f.HandleJSON("GET "+apiBase+"/drafts/d2", 200, metadataDraft("d2", "t2", "Re: Hello", "返信", 1700000002000, hdr("In-Reply-To", "<m1@x.example>")))

	got, err := e.c.ListDrafts(context.Background(), testAccount, 100)
	require.NoError(t, err)
	assert.False(t, got.Truncated)
	require.Len(t, got.Items, 2)

	d1, d2 := got.Items[0], got.Items[1]
	assert.Equal(t, "d1", d1.ID)
	assert.Equal(t, "t1", d1.ThreadID)
	assert.Equal(t, int64(1700000001000), d1.UpdatedAt.UnixMilli())
	assert.Equal(t, "新規の件名", d1.Subject)
	assert.Equal(t, `It's a "draft" & more`, d1.Snippet)
	require.Len(t, d1.To, 2)
	assert.Equal(t, "山田", d1.To[0].Name)
	assert.Equal(t, "yamada@example.com", d1.To[0].Email)
	assert.False(t, d1.IsReply)
	assert.True(t, d2.IsReply)

	for _, id := range []string{"d1", "d2"} {
		assert.Equal(t, "metadata", e.f.Requests("GET", apiBase+"/drafts/"+id)[0].Query.Get("format"), "本文を読まない形式で取得する")
	}
}

// 上限を超える下書きは、上限の件数だけ読んで Truncated を立てる（超過分の drafts.get は呼ばない）。
func TestListDraftsTruncatesAtLimit(t *testing.T) {
	e := newTestEnv(t)
	e.f.Handle("GET "+apiBase+"/drafts", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "3", r.URL.Query().Get("maxResults"), "超過の判定用に limit+1 件だけ求める")
		var ds []map[string]any
		for i := 1; i <= 3; i++ {
			ds = append(ds, map[string]any{"id": fmt.Sprintf("d%d", i), "message": map[string]any{"id": "m", "threadId": "t"}})
		}
		writeJSON(w, 200, map[string]any{"drafts": ds})
	})
	for _, id := range []string{"d1", "d2"} {
		e.f.HandleJSON("GET "+apiBase+"/drafts/"+id, 200, metadataDraft(id, "t", "s", "", 1700000000000))
	}

	got, err := e.c.ListDrafts(context.Background(), testAccount, 2)
	require.NoError(t, err)
	assert.True(t, got.Truncated)
	require.Len(t, got.Items, 2)
	assert.Empty(t, e.f.Requests("GET", apiBase+"/drafts/d3"))
}

// ページをまたいで集める。一覧のあとで削除された下書き（404）は含めず、ほかの失敗は一覧全体の失敗にする。
func TestListDraftsPagesAndSkipsDeleted(t *testing.T) {
	e := newTestEnv(t)
	e.f.Handle("GET "+apiBase+"/drafts", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("pageToken") {
		case "":
			writeJSON(w, 200, map[string]any{"nextPageToken": "p2", "drafts": []map[string]any{{"id": "d1", "message": map[string]any{"id": "m", "threadId": "t"}}}})
		case "p2":
			writeJSON(w, 200, map[string]any{"drafts": []map[string]any{{"id": "d2", "message": map[string]any{"id": "m", "threadId": "t"}}, {"id": "d3", "message": map[string]any{"id": "m", "threadId": "t"}}}})
		default:
			t.Errorf("想定外の pageToken")
		}
	})
	e.f.HandleJSON("GET "+apiBase+"/drafts/d1", 200, metadataDraft("d1", "t", "one", "", 1700000000000))
	e.f.HandleJSON("GET "+apiBase+"/drafts/d2", 404, apiError(404, "notFound", "gone"))
	e.f.HandleJSON("GET "+apiBase+"/drafts/d3", 200, metadataDraft("d3", "t", "three", "", 1700000003000))

	got, err := e.c.ListDrafts(context.Background(), testAccount, 100)
	require.NoError(t, err)
	require.Len(t, got.Items, 2)
	assert.Equal(t, []string{"d1", "d3"}, []string{got.Items[0].ID, got.Items[1].ID})
}

func TestListDraftsFailsWhenAnyReadFails(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/drafts", 200, map[string]any{"drafts": []map[string]any{
		{"id": "d1", "message": map[string]any{"id": "m", "threadId": "t"}}, {"id": "d2", "message": map[string]any{"id": "m", "threadId": "t"}},
	}})
	e.f.HandleJSON("GET "+apiBase+"/drafts/d1", 200, metadataDraft("d1", "t", "one", "", 1700000000000))
	e.f.HandleJSON("GET "+apiBase+"/drafts/d2", 401, apiError(401, "authError", "bad"))

	_, err := e.c.ListDrafts(context.Background(), testAccount, 100)
	require.ErrorIs(t, err, gmail.ErrNeedsReauth)
}

func TestListDraftsEmpty(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/drafts", 200, map[string]any{"resultSizeEstimate": 0})
	got, err := e.c.ListDrafts(context.Background(), testAccount, 100)
	require.NoError(t, err)
	assert.NotNil(t, got.Items)
	assert.Empty(t, got.Items)
	assert.False(t, got.Truncated)
}
