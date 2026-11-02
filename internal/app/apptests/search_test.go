package apptests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

func TestSearchMergesAccountsAndPages(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	e.PutRemoteThread(a.ID, "alpha1", apptest.Msg("m1", "alpha1", at(1), "SENT"))
	e.PutRemoteThread(a.ID, "alpha3", apptest.Msg("m3", "alpha3", at(3), "INBOX", "UNREAD"))
	e.PutRemoteThread(b.ID, "alpha2", apptest.Msg("m2", "alpha2", at(2), "TRASH"))
	e.PutRemoteThread(b.ID, "alpha4", apptest.Msg("m4", "alpha4", at(4), "INBOX"))
	e.PutRemoteThread(b.ID, "other", apptest.Msg("m5", "other", at(5), "INBOX"))
	ctx := t.Context()

	p, err := svc.Search(ctx, dto.SearchQuery{Q: " alpha ", Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha4", "alpha3"}, threadIDs(p.Items))
	assert.True(t, p.Items[1].IsUnread)
	assert.True(t, p.Items[1].InInbox)
	require.NotEmpty(t, p.NextCursor)

	p, err = svc.Search(ctx, dto.SearchQuery{Q: "alpha", Limit: 2, Cursor: p.NextCursor})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha2", "alpha1"}, threadIDs(p.Items), "INBOX 以外も対象")
	assert.Empty(t, p.NextCursor)

	_, err = e.Store.GetThread(ctx, a.ID, "alpha3")
	assert.ErrorIs(t, err, store.ErrNotFound, "検索結果は DB に保存しない")
	assert.Zero(t, e.Sync.FetchMoreCalls)
}

func TestSearchFollowsPageTokens(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	for i := 1; i <= 3; i++ {
		id := "hit" + string(rune('0'+i))
		e.PutRemoteThread(a.ID, id, apptest.Msg("m"+id, id, at(i), "INBOX"))
	}
	var got []string
	cursor := ""
	for range 5 {
		p, err := svc.Search(t.Context(), dto.SearchQuery{Q: "hit", Limit: 1, Cursor: cursor})
		require.NoError(t, err)
		got = append(got, threadIDs(p.Items)...)
		if cursor = p.NextCursor; cursor == "" {
			break
		}
	}
	assert.Equal(t, []string{"hit3", "hit2", "hit1"}, got)
}

func TestSearchAccountFilterFailureAndLabels(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	require.NoError(t, e.Store.ReplaceLabels(t.Context(), a.ID, []store.Label{
		{ID: "Label_1", Name: "Work", Type: "user", ColorBg: "#fff"}, {ID: "INBOX", Name: "INBOX", Type: "system"},
	}))
	e.PutRemoteThread(a.ID, "hit1", apptest.Msg("m1", "hit1", at(1), "INBOX", "Label_1"))
	e.PutRemoteThread(b.ID, "hit2", apptest.Msg("m2", "hit2", at(2), "INBOX"))

	p, err := svc.Search(t.Context(), dto.SearchQuery{Q: "hit", AccountIDs: []string{a.ID}})
	require.NoError(t, err)
	require.Len(t, p.Items, 1)
	require.Len(t, p.Items[0].Labels, 1)
	assert.Equal(t, "Work", p.Items[0].Labels[0].Name, "ユーザーラベル名を解決（システムラベルは含めない）")

	e.Gmail.Err["ListThreads:"+b.ID] = gmail.ErrRateLimited
	p, err = svc.Search(t.Context(), dto.SearchQuery{Q: "hit"})
	require.NoError(t, err)
	assert.Equal(t, []string{"hit1"}, threadIDs(p.Items))
	require.Len(t, p.Failures, 1)
	assert.Equal(t, b.ID, p.Failures[0].AccountID)
	assert.Equal(t, apperr.RateLimited, p.Failures[0].Error["code"])
}

func TestSearchValidation(t *testing.T) {
	svc, _ := newInboxSvc(t)
	for _, q := range []dto.SearchQuery{{Q: ""}, {Q: "   "}, {Q: "x", Limit: 101}, {Q: "x", Cursor: "%%"}} {
		_, err := svc.Search(t.Context(), q)
		_ = requireCode(t, apperr.Validation, err)
	}
	p, err := svc.Search(t.Context(), dto.SearchQuery{Q: "x"})
	require.NoError(t, err)
	assert.NotNil(t, p.Items)
	assert.NotNil(t, p.Failures)
}

func TestSearchIgnoresDraftsInSummary(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	m1 := apptest.Msg("m1", "hit1", at(1), "INBOX")
	m2 := apptest.Msg("m2", "hit1", at(2), "INBOX", "UNREAD")
	draft := apptest.Msg("md", "hit1", at(9), "DRAFT", "UNREAD")
	draft.From = model.Address{Email: "a@x.example"}
	draft.Snippet = "draft text"
	e.PutRemoteThread(a.ID, "hit1", m1, m2, draft)
	onlyDraft := apptest.Msg("md2", "hit2", at(10), "DRAFT")
	e.PutRemoteThread(a.ID, "hit2", onlyDraft)

	p, err := svc.Search(t.Context(), dto.SearchQuery{Q: "hit"})
	require.NoError(t, err)
	require.Len(t, p.Items, 1, "下書きだけのスレッドは結果に出さない")
	it := p.Items[0]
	assert.Equal(t, 2, it.MessageCount)
	assert.Equal(t, at(2).UnixMilli(), it.LastMessageAt)
	assert.Equal(t, "snippet m2", it.Snippet)
	assert.Equal(t, "sender@example.com", it.LastFrom.Email)
	assert.True(t, it.IsUnread)
}

func TestSearchFailedAccountKeepsStateInCursorAndRetries(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	e.PutRemoteThread(a.ID, "hit1", apptest.Msg("m1", "hit1", at(1), "INBOX"))
	e.PutRemoteThread(a.ID, "hit3", apptest.Msg("m3", "hit3", at(3), "INBOX"))
	e.PutRemoteThread(b.ID, "hit2", apptest.Msg("m2", "hit2", at(2), "INBOX"))
	e.PutRemoteThread(b.ID, "hit4", apptest.Msg("m4", "hit4", at(4), "INBOX"))
	ctx := t.Context()

	p, err := svc.Search(ctx, dto.SearchQuery{Q: "hit", Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, []string{"hit4"}, threadIDs(p.Items))
	require.NotEmpty(t, p.NextCursor)

	// 2 ページ目: B の ListThreads が一度だけ失敗する。
	e.Gmail.Err["ListThreads:"+b.ID] = gmail.ErrRateLimited
	p, err = svc.Search(ctx, dto.SearchQuery{Q: "hit", Limit: 1, Cursor: p.NextCursor})
	require.NoError(t, err)
	delete(e.Gmail.Err, "ListThreads:"+b.ID)
	assert.Equal(t, []string{"hit3"}, threadIDs(p.Items))
	require.Len(t, p.Failures, 1)
	assert.Equal(t, b.ID, p.Failures[0].AccountID)
	require.NotEmpty(t, p.NextCursor)
	states, err := appcore.DecodeSearchCursor(p.NextCursor)
	require.NoError(t, err)
	require.Contains(t, states, b.ID, "失敗した B の状態を続きのカーソルに残す")
	assert.Equal(t, "1", states[b.ID].PT, "B は入ってきた pageToken から再試行する")

	// 以降は B の結果も返る（時刻順を保つ）。
	var got []string
	cursor := p.NextCursor
	for range 5 {
		p, err = svc.Search(ctx, dto.SearchQuery{Q: "hit", Limit: 1, Cursor: cursor})
		require.NoError(t, err)
		assert.Empty(t, p.Failures)
		got = append(got, threadIDs(p.Items)...)
		if cursor = p.NextCursor; cursor == "" {
			break
		}
	}
	assert.Equal(t, []string{"hit2", "hit1"}, got)
}

func TestSearchFailureOnFirstPageIsRetriedFromStart(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	e.PutRemoteThread(a.ID, "hit1", apptest.Msg("m1", "hit1", at(1), "INBOX"))
	e.PutRemoteThread(b.ID, "hit2", apptest.Msg("m2", "hit2", at(2), "INBOX"))
	ctx := t.Context()

	e.Gmail.Err["ListThreads:"+b.ID] = gmail.ErrRateLimited
	p, err := svc.Search(ctx, dto.SearchQuery{Q: "hit", Limit: 5})
	require.NoError(t, err)
	delete(e.Gmail.Err, "ListThreads:"+b.ID)
	assert.Equal(t, []string{"hit1"}, threadIDs(p.Items))
	require.Len(t, p.Failures, 1)
	require.NotEmpty(t, p.NextCursor, "先頭で失敗したアカウントの再試行を続きのカーソルで表す")

	p, err = svc.Search(ctx, dto.SearchQuery{Q: "hit", Limit: 5, Cursor: p.NextCursor})
	require.NoError(t, err)
	assert.Equal(t, []string{"hit2"}, threadIDs(p.Items))
	assert.Empty(t, p.Failures)
	assert.Empty(t, p.NextCursor)
}

func TestSearchMetadataFailureKeepsPageForRetry(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutRemoteThread(a.ID, "hit1", apptest.Msg("m1", "hit1", at(1), "INBOX"))
	e.PutRemoteThread(a.ID, "hit2", apptest.Msg("m2", "hit2", at(2), "INBOX"))
	ctx := t.Context()

	e.Gmail.Err["GetThreadMetadata:"+a.ID+":hit1"] = gmail.ErrRateLimited
	p, err := svc.Search(ctx, dto.SearchQuery{Q: "hit", Limit: 5})
	require.NoError(t, err)
	delete(e.Gmail.Err, "GetThreadMetadata:"+a.ID+":hit1")
	assert.Empty(t, p.Items)
	require.Len(t, p.Failures, 1)
	require.NotEmpty(t, p.NextCursor)

	p, err = svc.Search(ctx, dto.SearchQuery{Q: "hit", Limit: 5, Cursor: p.NextCursor})
	require.NoError(t, err)
	assert.Equal(t, []string{"hit2", "hit1"}, threadIDs(p.Items))
	assert.Empty(t, p.NextCursor)
}
