package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/store"
)

func threadKeys(rows []store.ThreadRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.AccountID+"/"+r.ThreadID)
	}
	return out
}

// pageAll は limit 件ずつキーセットカーソルで全件をたどる。
func pageAll(t *testing.T, s *testStore, f store.InboxFilter) []store.ThreadRow {
	t.Helper()
	var all []store.ThreadRow
	for range 100 {
		rows, err := s.ListInbox(context.Background(), f)
		require.NoError(t, err)
		if len(rows) == 0 {
			return all
		}
		all = append(all, rows...)
		last := rows[len(rows)-1]
		f.After = &store.InboxCursor{LastMessageAt: last.LastMessageAt, AccountID: last.AccountID, ThreadID: last.ThreadID}
	}
	t.Fatal("paging did not terminate")
	return nil
}

func TestListInboxKeysetPagingTieBreak(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	b := seedAccount(t, s, "b@example.com")
	lo, hi := a, b
	if lo.ID > hi.ID {
		lo, hi = hi, lo
	}
	t1 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	// t2 に 4 件（同一時刻）、t1 に 3 件。account_id → id の昇順がタイブレーク。
	for _, id := range []string{"t3", "t1", "t2"} {
		apply(t, s, snap(hi.ID, id, t2, "INBOX"))
	}
	apply(t, s, snap(lo.ID, "t9", t2, "INBOX"))
	for _, id := range []string{"x2", "x1"} {
		apply(t, s, snap(lo.ID, id, t1, "INBOX"))
	}
	apply(t, s, snap(hi.ID, "x0", t1, "INBOX"))
	apply(t, s, snap(hi.ID, "archived", t2)) // 受信箱外は出ない

	want := []string{
		lo.ID + "/t9", hi.ID + "/t1", hi.ID + "/t2", hi.ID + "/t3",
		lo.ID + "/x1", lo.ID + "/x2", hi.ID + "/x0",
	}
	all, err := s.ListInbox(ctx, store.InboxFilter{Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, want, threadKeys(all))

	// どのページサイズでも、ページをまたいで欠落・重複なく同じ順序になる。
	for limit := 1; limit <= 4; limit++ {
		assert.Equal(t, want, threadKeys(pageAll(t, s, store.InboxFilter{Limit: limit})), "limit=%d", limit)
	}
	// 同一時刻の途中（hi/t1 の次）から再開する。
	rows, err := s.ListInbox(ctx, store.InboxFilter{Limit: 2, After: &store.InboxCursor{LastMessageAt: t2, AccountID: hi.ID, ThreadID: "t1"}})
	require.NoError(t, err)
	assert.Equal(t, []string{hi.ID + "/t2", hi.ID + "/t3"}, threadKeys(rows))
}

func TestListInboxFilters(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	b := seedAccount(t, s, "b@example.com")
	c := seedAccount(t, s, "c@example.com")
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	apply(t, s, snap(a.ID, "a-read", at.Add(1*time.Hour), "INBOX"))
	apply(t, s, snap(a.ID, "a-unread", at.Add(2*time.Hour), "INBOX", "UNREAD"))
	apply(t, s, snap(b.ID, "b-unread", at.Add(3*time.Hour), "INBOX", "UNREAD"))
	apply(t, s, snap(c.ID, "c-read", at.Add(4*time.Hour), "INBOX"))

	rows, err := s.ListInbox(ctx, store.InboxFilter{AccountIDs: []string{a.ID, b.ID}, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{b.ID + "/b-unread", a.ID + "/a-unread", a.ID + "/a-read"}, threadKeys(rows))

	rows, err = s.ListInbox(ctx, store.InboxFilter{AccountIDs: []string{a.ID}, UnreadOnly: true, Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, []string{a.ID + "/a-unread"}, threadKeys(rows))

	rows, err = s.ListInbox(ctx, store.InboxFilter{UnreadOnly: true})
	require.NoError(t, err)
	assert.Equal(t, []string{b.ID + "/b-unread", a.ID + "/a-unread"}, threadKeys(rows), "Limit 未指定でも既定件数で返る")

	rows, err = s.ListInbox(ctx, store.InboxFilter{AccountIDs: []string{"nobody"}, Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.NotNil(t, rows, "空でも nil にしない")

	// 絞り込み＋ページングの併用。
	rows, err = s.ListInbox(ctx, store.InboxFilter{AccountIDs: []string{a.ID, c.ID}, Limit: 1})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, c.ID+"/c-read", threadKeys(rows)[0])
}

func TestListInboxAttachesUserLabels(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	b := seedAccount(t, s, "b@example.com")
	for _, acc := range []store.Account{a, b} {
		require.NoError(t, s.ReplaceLabels(ctx, acc.ID, []store.Label{
			{ID: "INBOX", Name: "INBOX", Type: "system"},
			{ID: "Label_2", Name: "B 案件", Type: "user", ColorBg: "#000000", ColorFg: "#ffffff"},
			{ID: "Label_1", Name: "A 案件", Type: "user"},
		}))
	}
	at := clk.Now()
	sn := snap(a.ID, "t", at, "INBOX", "Label_2")
	sn.Messages = append(sn.Messages, msg("t-m2", at.Add(time.Minute), "INBOX", "Label_1", "Label_2", "Label_unknown"))
	apply(t, s, sn)
	apply(t, s, snap(b.ID, "t", at.Add(time.Hour), "INBOX")) // 同じスレッド ID でも別アカウント

	rows, err := s.ListInbox(ctx, store.InboxFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Empty(t, rows[0].Labels)
	assert.NotNil(t, rows[0].Labels)
	assert.Equal(t, []store.Label{
		{ID: "Label_1", Name: "A 案件", Type: "user"},
		{ID: "Label_2", Name: "B 案件", Type: "user", ColorBg: "#000000", ColorFg: "#ffffff"},
	}, rows[1].Labels, "ユーザーラベルのみ・重複なし・名前順")

	one, err := s.GetThread(ctx, a.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, rows[1], one)
}

func TestUnreadCounts(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	b := seedAccount(t, s, "b@example.com")
	seedAccount(t, s, "c@example.com")
	at := clk.Now()

	counts, err := s.UnreadCounts(ctx)
	require.NoError(t, err)
	assert.Empty(t, counts)
	assert.NotNil(t, counts)

	apply(t, s, snap(a.ID, "u1", at, "INBOX", "UNREAD"))
	apply(t, s, snap(a.ID, "u2", at, "INBOX", "UNREAD"))
	apply(t, s, snap(a.ID, "r1", at, "INBOX"))
	apply(t, s, snap(a.ID, "arch", at, "UNREAD")) // 受信箱外の未読は数えない
	apply(t, s, snap(b.ID, "u1", at, "INBOX", "UNREAD"))
	counts, err = s.UnreadCounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{a.ID: 2, b.ID: 1}, counts, "未読 0 のアカウントは含めない")

	apply(t, s, snap(a.ID, "u1", at, "INBOX")) // 既読化
	counts, err = s.UnreadCounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{a.ID: 1, b.ID: 1}, counts)
}

func TestListInboxTabs(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	b := seedAccount(t, s, "b@example.com")
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	apply(t, s, snap(a.ID, "personal", at.Add(1*time.Hour), "INBOX", "CATEGORY_PERSONAL"))
	apply(t, s, snap(a.ID, "social-unread", at.Add(2*time.Hour), "INBOX", "CATEGORY_SOCIAL", "UNREAD"))
	apply(t, s, snap(b.ID, "promo", at.Add(3*time.Hour), "INBOX", "CATEGORY_PROMOTIONS"))
	apply(t, s, snap(b.ID, "updates", at.Add(4*time.Hour), "INBOX", "CATEGORY_UPDATES"))
	apply(t, s, snap(b.ID, "nocat", at.Add(5*time.Hour), "INBOX"))
	apply(t, s, snap(b.ID, "out", at.Add(6*time.Hour), "CATEGORY_SOCIAL")) // 受信箱外は出ない
	// 複数メッセージのスレッド: どれかのメッセージにカテゴリがあればそのカテゴリのスレッド（和集合）。
	multi := snap(a.ID, "multi", at.Add(7*time.Hour), "INBOX", "CATEGORY_PROMOTIONS")
	multi.Messages = append(multi.Messages, msg("multi-m2", at.Add(7*time.Hour+time.Minute), "INBOX"))
	apply(t, s, multi)

	keys := func(f store.InboxFilter) []string {
		f.Limit = 100
		rows, err := s.ListInbox(ctx, f)
		require.NoError(t, err)
		return threadKeys(rows)
	}
	assert.Equal(t, []string{a.ID + "/multi", b.ID + "/promo"}, keys(store.InboxFilter{Tab: store.InboxTabPromotions}))
	assert.Equal(t, []string{a.ID + "/social-unread"}, keys(store.InboxFilter{Tab: store.InboxTabSocial}))
	assert.Equal(t, []string{b.ID + "/nocat", a.ID + "/personal"}, keys(store.InboxFilter{Tab: store.InboxTabPrimary}))
	assert.Equal(t, []string{a.ID + "/social-unread"}, keys(store.InboxFilter{Tab: store.InboxTabNew}))
	assert.Len(t, keys(store.InboxFilter{Tab: store.InboxTabAll}), 6)
	// アカウント別。
	assert.Equal(t, []string{b.ID + "/promo"}, keys(store.InboxFilter{Tab: store.InboxTabPromotions, AccountIDs: []string{b.ID}}))
	assert.Equal(t, []string{b.ID + "/nocat"}, keys(store.InboxFilter{Tab: store.InboxTabPrimary, AccountIDs: []string{b.ID}}))
	assert.Empty(t, keys(store.InboxFilter{Tab: store.InboxTabNew, AccountIDs: []string{b.ID}}))
	// 未読のみとの併用は AND。
	assert.Empty(t, keys(store.InboxFilter{Tab: store.InboxTabPromotions, UnreadOnly: true}))
	// ページングはタブの絞り込み後の順序で欠落・重複しない。
	assert.Equal(t, []string{a.ID + "/multi", b.ID + "/promo"}, threadKeys(pageAll(t, s, store.InboxFilter{Tab: store.InboxTabPromotions, Limit: 1})))

	assert.True(t, store.InboxTabNew.Valid())
	assert.False(t, store.InboxTab("updates").Valid())
}
