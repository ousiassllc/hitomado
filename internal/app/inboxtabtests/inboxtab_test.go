// Package inboxtabtests は ListInbox の受信箱タブ（すべて・メイン・ソーシャル・プロモーション・新着）の絞り込みを、
// 実 SQLite・フェイク（apptest）で検証する外部テスト。
package inboxtabtests

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
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

func requireValidation(t *testing.T, err error) {
	t.Helper()
	var ae *apperr.Error
	require.True(t, errors.As(err, &ae), "apperr.Error ではない: %v", err)
	require.Equal(t, apperr.Validation, ae.Code)
}

// 各タブで該当ラベルのスレッドだけが、全アカウント統合でもアカウント別でも最終メッセージ日時の降順で出る。
func TestListInboxTabs(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	e.PutThread(t, a.ID, "a-personal", apptest.Msg("m1", "a-personal", at(1), "INBOX", "CATEGORY_PERSONAL"))
	e.PutThread(t, a.ID, "a-social", apptest.Msg("m2", "a-social", at(2), "INBOX", "CATEGORY_SOCIAL", "UNREAD"))
	e.PutThread(t, a.ID, "a-promo", apptest.Msg("m3", "a-promo", at(3), "INBOX", "CATEGORY_PROMOTIONS"))
	e.PutThread(t, a.ID, "a-updates", apptest.Msg("m4", "a-updates", at(4), "INBOX", "CATEGORY_UPDATES", "UNREAD"))
	e.PutThread(t, b.ID, "b-nocat", apptest.Msg("m5", "b-nocat", at(5), "INBOX"))
	e.PutThread(t, b.ID, "b-promo", apptest.Msg("m6", "b-promo", at(6), "INBOX", "CATEGORY_PROMOTIONS", "UNREAD"))
	e.PutThread(t, b.ID, "b-personal", apptest.Msg("m7", "b-personal", at(7), "INBOX", "CATEGORY_PERSONAL"))
	e.PutThread(t, b.ID, "b-archived", apptest.Msg("m8", "b-archived", at(8), "CATEGORY_PERSONAL", "UNREAD"))
	exhaust(t, e, a.ID, b.ID)
	ctx := t.Context()

	cases := []struct {
		tab        string
		all, onlyA []string
	}{
		{"", []string{"b-personal", "b-promo", "b-nocat", "a-updates", "a-promo", "a-social", "a-personal"}, []string{"a-updates", "a-promo", "a-social", "a-personal"}},
		{"all", []string{"b-personal", "b-promo", "b-nocat", "a-updates", "a-promo", "a-social", "a-personal"}, []string{"a-updates", "a-promo", "a-social", "a-personal"}},
		// カテゴリを持たない受信箱のスレッドはメインに出る。「更新」だけのスレッドはどのカテゴリのタブにも出ない。
		{"primary", []string{"b-personal", "b-nocat", "a-personal"}, []string{"a-personal"}},
		{"social", []string{"a-social"}, []string{"a-social"}},
		{"promotions", []string{"b-promo", "a-promo"}, []string{"a-promo"}},
		{"new", []string{"b-promo", "a-updates", "a-social"}, []string{"a-updates", "a-social"}},
	}
	for _, c := range cases {
		p, err := svc.ListInbox(ctx, dto.InboxQuery{Tab: c.tab})
		require.NoError(t, err, c.tab)
		assert.Equal(t, c.all, threadIDs(p.Items), "全アカウント tab=%q", c.tab)
		p, err = svc.ListInbox(ctx, dto.InboxQuery{Tab: c.tab, AccountIDs: []string{a.ID}})
		require.NoError(t, err, c.tab)
		assert.Equal(t, c.onlyA, threadIDs(p.Items), "アカウント別 tab=%q", c.tab)
	}

	// タブとページングの併用（キーセットは絞り込み後の順序で続く）。
	p, err := svc.ListInbox(ctx, dto.InboxQuery{Tab: "primary", Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"b-personal", "b-nocat"}, threadIDs(p.Items))
	require.NotEmpty(t, p.NextCursor)
	p, err = svc.ListInbox(ctx, dto.InboxQuery{Tab: "primary", Limit: 2, Cursor: p.NextCursor})
	require.NoError(t, err)
	assert.Equal(t, []string{"a-personal"}, threadIDs(p.Items))
	assert.Empty(t, p.NextCursor)

	_, err = svc.ListInbox(ctx, dto.InboxQuery{Tab: "updates"})
	requireValidation(t, err)
}

// 新着タブは未読だけが出て、既読にすると次の取得で消え、未読に戻すと戻る。
func TestListInboxNewTabDropsThreadOnceRead(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX", "UNREAD"))
	e.PutThread(t, a.ID, "t2", apptest.Msg("m2", "t2", at(2), "INBOX", "UNREAD"))
	exhaust(t, e, a.ID)
	ctx := t.Context()

	p, err := svc.ListInbox(ctx, dto.InboxQuery{Tab: "new"})
	require.NoError(t, err)
	assert.Equal(t, []string{"t2", "t1"}, threadIDs(p.Items))

	_, err = svc.MarkRead(ctx, []dto.ThreadRef{{AccountID: a.ID, ThreadID: "t2"}}, true)
	require.NoError(t, err)
	p, err = svc.ListInbox(ctx, dto.InboxQuery{Tab: "new"})
	require.NoError(t, err)
	assert.Equal(t, []string{"t1"}, threadIDs(p.Items))
	p, err = svc.ListInbox(ctx, dto.InboxQuery{})
	require.NoError(t, err)
	assert.Equal(t, []string{"t2", "t1"}, threadIDs(p.Items), "すべてのタブには既読も残る")
}

// タブの絞り込みはローカルのキャッシュにだけ掛かり、Gmail 側の続きがある間は INBOX の続きを追加取得して探す。
func TestListInboxTabFetchesMoreFromGmail(t *testing.T) {
	svc, e := newInboxSvc(t)
	a := e.AddAccount(t, "a@x.example")
	for i := 1; i <= 6; i++ {
		id := fmt.Sprintf("t%d", i)
		labels := []string{"INBOX"}
		if i%3 == 0 {
			labels = append(labels, "CATEGORY_SOCIAL")
		}
		e.PutRemoteThread(a.ID, id, apptest.Msg("m"+id, id, at(100-i), labels...))
	}
	e.Sync.PageSize = 2
	ctx := t.Context()

	p, err := svc.ListInbox(ctx, dto.InboxQuery{Tab: "social", Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"t3", "t6"}, threadIDs(p.Items))
	assert.Empty(t, p.NextCursor)
}
