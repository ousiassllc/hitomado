package apptests

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/store"
)

// 条件（未読のみ）に合う行が Gmail の先の方にしか無いとき、1 回の呼び出しで見つからなくても続きのカーソルを返す。
func TestListInboxUnreadOnlyContinuesWhenNoRowsMatchYet(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	for i := 1; i <= 14; i++ {
		id := fmt.Sprintf("t%02d", i)
		labels := []string{"INBOX"}
		if i == 14 {
			labels = append(labels, "UNREAD") // 最後（Gmail の末尾側）にだけ未読
		}
		e.PutRemoteThread(a.ID, id, apptest.Msg("m"+id, id, at(100-i), labels...))
	}
	e.Sync.PageSize = 2
	ctx := t.Context()

	p, err := svc.ListInbox(ctx, dto.InboxQuery{UnreadOnly: true, Limit: 5})
	require.NoError(t, err)
	assert.Empty(t, p.Items, "最初の 5 周（10 件）には未読がない")
	require.NotEmpty(t, p.NextCursor, "Gmail 側に続きがある限りカーソルを返す")
	assert.Len(t, e.Sync.FetchMoreCalls, 5)

	p, err = svc.ListInbox(ctx, dto.InboxQuery{UnreadOnly: true, Limit: 5, Cursor: p.NextCursor})
	require.NoError(t, err)
	assert.Equal(t, []string{"t14"}, threadIDs(p.Items))
	assert.Empty(t, p.NextCursor, "末尾に達した")
}

// 既知のスレッドだけのページ（added=0）でもトークンが進んでいれば取得を続ける。止まったら打ち切る。
func TestListInboxContinuesWhileTokenAdvances(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutThread(t, a.ID, "late", apptest.Msg("m1", "late", at(1), "INBOX"))
	ctx := t.Context()
	steps := 0
	e.Sync.FetchMoreFn = func(ctx context.Context, accountID string) (int, error) {
		steps++
		st, err := e.Store.GetSyncState(ctx, accountID)
		if err != nil {
			return 0, err
		}
		st.InboxPageToken = fmt.Sprintf("tok%d", steps)
		st.InboxExhausted = steps == 3
		return 0, e.Store.SaveSyncState(ctx, st)
	}

	p, err := svc.ListInbox(ctx, dto.InboxQuery{Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, []string{"late"}, threadIDs(p.Items))
	assert.Equal(t, 3, steps, "added=0 でもトークンが進む間は続け、末尾（exhausted）で止まる")
	assert.Empty(t, p.NextCursor)

	// 進まない（トークンも不変）ときは打ち切り、無限に続きを返さない。
	require.NoError(t, e.Store.SaveSyncState(ctx, store.SyncState{AccountID: a.ID, InboxPageToken: "stuck"}))
	steps = 0
	e.Sync.FetchMoreFn = func(context.Context, string) (int, error) { steps++; return 0, nil }
	p, err = svc.ListInbox(ctx, dto.InboxQuery{Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, steps)
	assert.Empty(t, p.NextCursor)
}
