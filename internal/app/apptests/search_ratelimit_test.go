package apptests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

// 一部のスレッドの threads.get だけが利用上限で失敗しても、取れたスレッドは出し、失敗はアカウント名つきで返せる形で残す（#392）。
func TestSearchPartialMetadataFailureStillReturnsFetchedThreads(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutRemoteThread(a.ID, "hit1", apptest.Msg("m1", "hit1", at(1), "INBOX"))
	e.PutRemoteThread(a.ID, "hit2", apptest.Msg("m2", "hit2", at(2), "INBOX"))
	e.PutRemoteThread(a.ID, "hit3", apptest.Msg("m3", "hit3", at(3), "INBOX"))
	ctx := t.Context()

	e.Gmail.Err["GetThreadMetadata:"+a.ID+":hit2"] = gmail.ErrRateLimited
	p, err := svc.Search(ctx, dto.SearchQuery{Q: "hit", Limit: 5})
	require.NoError(t, err)
	assert.Equal(t, []string{"hit3", "hit1"}, threadIDs(p.Items), "取れたスレッドは出す")
	require.Len(t, p.Failures, 1)
	assert.Equal(t, a.ID, p.Failures[0].AccountID)
	assert.Equal(t, apperr.RateLimited, p.Failures[0].Error["code"])
}

// 4 アカウントすべてで、どのスレッドも利用上限で取れなかったときは、空の成功ではなく全アカウントの失敗として返す（#392）。
func TestSearchAllAccountsRateLimitedReportsEveryAccount(t *testing.T) {
	svc, e := newInboxSvc(t)
	var ids []string
	for _, email := range []string{"a@x.example", "b@x.example", "c@x.example", "d@x.example"} {
		a := e.AddAccount(t, email)
		ids = append(ids, a.ID)
		e.PutRemoteThread(a.ID, "hit-"+email[:1], apptest.Msg("m"+email[:1], "hit-"+email[:1], at(1), "INBOX"))
		e.Gmail.Err["GetThreadMetadata:"+a.ID] = gmail.ErrRateLimited
	}
	p, err := svc.Search(t.Context(), dto.SearchQuery{Q: "hit", Limit: 5})
	require.NoError(t, err)
	assert.Empty(t, p.Items)
	var failed []string
	for _, f := range p.Failures {
		assert.Equal(t, apperr.RateLimited, f.Error["code"])
		failed = append(failed, f.AccountID)
	}
	assert.ElementsMatch(t, ids, failed)
	require.NotEmpty(t, p.NextCursor, "先頭からの再試行を続きのカーソルで表す")

	for _, id := range ids {
		delete(e.Gmail.Err, "GetThreadMetadata:"+id)
	}
	p, err = svc.Search(t.Context(), dto.SearchQuery{Q: "hit", Limit: 5, Cursor: p.NextCursor})
	require.NoError(t, err)
	assert.Len(t, p.Items, 4, "上限が解けたあとの再試行で 4 アカウントのメールが出る")
	assert.Empty(t, p.Failures)
}

// threads.list の historyId が変わっていないスレッドは、検索を繰り返しても threads.get を呼び直さない。変わったスレッドだけ取り直す（#392）。
func TestSearchReusesThreadMetadataWhileHistoryIDIsUnchanged(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutRemoteThread(a.ID, "hit1", apptest.Msg("m1", "hit1", at(1), "INBOX"))
	e.PutRemoteThread(a.ID, "hit2", apptest.Msg("m2", "hit2", at(2), "INBOX", "UNREAD"))
	ctx := t.Context()

	p, err := svc.Search(ctx, dto.SearchQuery{Q: "hit"})
	require.NoError(t, err)
	require.Equal(t, []string{"hit2", "hit1"}, threadIDs(p.Items))
	require.Equal(t, 2, e.Gmail.CallCount("GetThreadMetadata"))

	p, err = svc.Search(ctx, dto.SearchQuery{Q: "hit"})
	require.NoError(t, err)
	assert.Equal(t, []string{"hit2", "hit1"}, threadIDs(p.Items))
	assert.Equal(t, 2, e.Gmail.CallCount("GetThreadMetadata"), "変わっていないスレッドは Gmail に頼み直さない")

	// hit2 が既読になった（historyId が進む）。hit2 だけ取り直し、最新の内容を返す。
	key := apptest.Key(a.ID, "hit2")
	tf := e.Gmail.Threads[key]
	tf.HistoryID = "h-hit2-next"
	tf.Messages = []gmail.MessageFull{apptest.Msg("m2", "hit2", at(2), "INBOX")}
	e.Gmail.Threads[key] = tf
	p, err = svc.Search(ctx, dto.SearchQuery{Q: "hit"})
	require.NoError(t, err)
	assert.Equal(t, 3, e.Gmail.CallCount("GetThreadMetadata"), "historyId が変わった hit2 だけ取り直す")
	require.Len(t, p.Items, 2)
	assert.False(t, p.Items[0].IsUnread, "古い（未読の）内容を返さない")
}
