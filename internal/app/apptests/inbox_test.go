package apptests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

func newInboxSvc(t *testing.T) (*app.InboxService, *apptest.Env) {
	t.Helper()
	e := apptest.NewEnv(t)
	return app.NewInboxService(e.Deps), e
}

func at(minutes int) time.Time { return apptest.Epoch.Add(time.Duration(minutes) * time.Minute) }

func exhaust(t *testing.T, e *apptest.Env, ids ...string) {
	t.Helper()
	for _, id := range ids {
		require.NoError(t, e.Store.SaveSyncState(t.Context(), store.SyncState{AccountID: id, InboxExhausted: true, InitialSyncDone: true}))
	}
}

func threadIDs(items []dto.ThreadSummary) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ThreadID
	}
	return out
}

func TestListInboxPagesAcrossAccounts(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	for i := 1; i <= 5; i++ {
		id := fmt.Sprintf("a%d", i)
		e.PutThread(t, a.ID, id, apptest.Msg("m-"+id, id, at(i*2), "INBOX"))
	}
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("b%d", i)
		e.PutThread(t, b.ID, id, apptest.Msg("m-"+id, id, at(i*2+1), "INBOX", "UNREAD"))
	}
	exhaust(t, e, a.ID, b.ID)
	ctx := t.Context()

	var got []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		p, err := svc.ListInbox(ctx, dto.InboxQuery{Limit: 3, Cursor: cursor})
		require.NoError(t, err)
		assert.NotNil(t, p.Failures)
		got = append(got, threadIDs(p.Items)...)
		cursor = p.NextCursor
		if cursor == "" {
			break
		}
		assert.Len(t, p.Items, 3)
	}
	assert.Equal(t, []string{"a5", "a4", "b3", "a3", "b2", "a2", "b1", "a1"}, got, "最終メッセージ日時の降順で重複なく")
	assert.Zero(t, e.Sync.FetchMoreCalls, "取得済みのアカウントでは Gmail へ追加取得しない")
}

func TestListInboxFilters(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	e.PutThread(t, a.ID, "a1", apptest.Msg("m1", "a1", at(1), "INBOX", "UNREAD"))
	e.PutThread(t, a.ID, "a2", apptest.Msg("m2", "a2", at(2), "INBOX"))
	e.PutThread(t, b.ID, "b1", apptest.Msg("m3", "b1", at(3), "INBOX", "UNREAD"))
	e.PutThread(t, b.ID, "b2", apptest.Msg("m4", "b2", at(4), "SENT"))
	exhaust(t, e, a.ID, b.ID)
	ctx := t.Context()

	p, err := svc.ListInbox(ctx, dto.InboxQuery{UnreadOnly: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"b1", "a1"}, threadIDs(p.Items))
	assert.True(t, p.Items[0].IsUnread)

	p, err = svc.ListInbox(ctx, dto.InboxQuery{AccountIDs: []string{a.ID, "nope"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"a2", "a1"}, threadIDs(p.Items))

	p, err = svc.ListInbox(ctx, dto.InboxQuery{AccountIDs: []string{"nope"}})
	require.NoError(t, err)
	assert.NotNil(t, p.Items)
	assert.Empty(t, p.Items)
	assert.Empty(t, p.NextCursor)
}

func TestListInboxValidation(t *testing.T) {
	svc, _ := newInboxSvc(t)
	for _, q := range []dto.InboxQuery{{Limit: -1}, {Limit: 101}, {Cursor: "!!!"}, {Cursor: "e30"}} {
		_, err := svc.ListInbox(t.Context(), q)
		_ = requireCode(t, apperr.Validation, err)
	}
	p, err := svc.ListInbox(t.Context(), dto.InboxQuery{})
	require.NoError(t, err)
	assert.NotNil(t, p.Items)
	assert.NotNil(t, p.Failures)
}

func TestListInboxFetchesMoreFromGmail(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("t%d", i)
		e.PutRemoteThread(a.ID, id, apptest.Msg("m"+id, id, at(100-i), "INBOX"))
	}
	e.Sync.PageSize = 2
	ctx := t.Context()

	p, err := svc.ListInbox(ctx, dto.InboxQuery{Limit: 3})
	require.NoError(t, err)
	assert.Equal(t, []string{"t1", "t2", "t3"}, threadIDs(p.Items))
	require.NotEmpty(t, p.NextCursor)
	assert.Equal(t, []string{a.ID, a.ID}, e.Sync.FetchMoreCalls, "limit+1 件に届くまで追加取得")

	p, err = svc.ListInbox(ctx, dto.InboxQuery{Limit: 3, Cursor: p.NextCursor})
	require.NoError(t, err)
	assert.Equal(t, []string{"t4", "t5", "t6"}, threadIDs(p.Items))
	assert.Empty(t, p.NextCursor, "末尾に達したらカーソルなし")
	st, err := e.Store.GetSyncState(ctx, a.ID)
	require.NoError(t, err)
	assert.True(t, st.InboxExhausted)
}

func TestListInboxPerAccountFailure(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	e.PutThread(t, a.ID, "a1", apptest.Msg("m1", "a1", at(1), "INBOX"))
	exhaust(t, e, a.ID)
	e.Sync.FetchMoreErr[b.ID] = fmt.Errorf("list: %w", gmail.ErrNeedsReauth)

	p, err := svc.ListInbox(t.Context(), dto.InboxQuery{})
	require.NoError(t, err)
	assert.Equal(t, []string{"a1"}, threadIDs(p.Items))
	require.Len(t, p.Failures, 1)
	assert.Equal(t, b.ID, p.Failures[0].AccountID)
	assert.Equal(t, apperr.NeedsReauth, p.Failures[0].Error["code"])
	assert.Equal(t, []string{b.ID}, e.Sync.FetchMoreCalls, "失敗したアカウントは再試行しない")

	got, _ := e.Store.GetAccount(t.Context(), b.ID)
	assert.Equal(t, store.StatusNeedsReauth, got.Status)
	assert.Len(t, e.Events.Named(events.AccountStatus), 1)
}

func TestRefresh(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	e.Sync.RefreshErr[b.ID] = gmail.ErrOffline

	r, err := svc.Refresh(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, r.Results, 2)
	assert.Equal(t, dto.RefreshResult{AccountID: a.ID, OK: true}, r.Results[0])
	assert.False(t, r.Results[1].OK)
	assert.Equal(t, apperr.Offline, r.Results[1].Error["code"])
}

func TestGetUnreadCounts(t *testing.T) {
	svc, e := newInboxSvc(t)
	m, err := svc.GetUnreadCounts(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, m)
	a := e.AddAccount(t, "a@x.example")
	e.PutThread(t, a.ID, "a1", apptest.Msg("m1", "a1", at(1), "INBOX", "UNREAD"))
	e.PutThread(t, a.ID, "a2", apptest.Msg("m2", "a2", at(2), "INBOX"))
	m, err = svc.GetUnreadCounts(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[string]int{a.ID: 1}, m)
}
