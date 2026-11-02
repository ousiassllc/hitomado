package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store"
)

func TestApplyThreadAggregates(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	res := apply(t, s, store.ThreadSnapshot{
		AccountID: a.ID, ThreadID: "t1", HistoryID: "h9", Subject: "最初の件名", HasAttachment: true,
		Messages: []store.MessageSnapshot{
			msg("m1", t0, "INBOX"),
			msg("m2", t0.Add(time.Hour), "INBOX", "UNREAD", "CATEGORY_PROMOTIONS"),
			msg("m3", t0.Add(2*time.Hour), "SENT"),
		},
	})
	assert.Equal(t, []string{"m1", "m2", "m3"}, res.NewMessageIDs)

	row, err := s.GetThread(ctx, a.ID, "t1")
	require.NoError(t, err)
	assert.Equal(t, "最初の件名", row.Subject)
	assert.Equal(t, "snippet m3", row.Snippet, "最新メッセージのスニペット")
	assert.Equal(t, t0.Add(2*time.Hour), row.LastMessageAt)
	assert.Equal(t, "From m3", row.LastFrom.Name)
	assert.Equal(t, "from-m3@example.com", row.LastFrom.Email, "差出人アドレスは小文字化")
	assert.Equal(t, 3, row.MessageCount)
	assert.True(t, row.IsUnread, "いずれかのメッセージが UNREAD")
	assert.True(t, row.HasAttachment)
	assert.True(t, row.InInbox, "いずれかのメッセージが INBOX")
	assert.Equal(t, 5, count(t, s, `SELECT COUNT(*) FROM message_labels`), "SENT/INBOX×2/UNREAD/CATEGORY")
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM threads WHERE left_inbox_at IS NOT NULL`))

	// 再適用: 既知のメッセージは新規扱いにならず、未読が解除され、新メッセージだけが返る。
	res = apply(t, s, store.ThreadSnapshot{
		AccountID: a.ID, ThreadID: "t1", HistoryID: "h10", Subject: "最初の件名",
		Messages: []store.MessageSnapshot{
			msg("m1", t0, "INBOX"),
			msg("m2", t0.Add(time.Hour), "INBOX"),
			msg("m3", t0.Add(2*time.Hour), "SENT"),
			msg("m4", t0.Add(3*time.Hour), "INBOX"),
		},
	})
	assert.Equal(t, []string{"m4"}, res.NewMessageIDs)
	row, err = s.GetThread(ctx, a.ID, "t1")
	require.NoError(t, err)
	assert.Equal(t, 4, row.MessageCount)
	assert.False(t, row.IsUnread)
	assert.False(t, row.HasAttachment, "has_attachment はスナップショットの値に追従")
	assert.Equal(t, 1, count(t, s, `SELECT COUNT(*) FROM threads`))
	assert.Equal(t, 4, count(t, s, `SELECT COUNT(*) FROM messages`))

	// 消えたメッセージは置き換えで取り除かれる（message_labels もカスケード）。
	apply(t, s, snap(a.ID, "t1", t0, "INBOX"))
	assert.Equal(t, 1, count(t, s, `SELECT COUNT(*) FROM messages`))
	assert.Equal(t, 1, count(t, s, `SELECT COUNT(*) FROM message_labels`))
}

func TestApplyThreadSameInternalDatePicksLaterMessage(t *testing.T) {
	s, _ := newTestStore(t)
	a := seedAccount(t, s, "a@example.com")
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	apply(t, s, store.ThreadSnapshot{AccountID: a.ID, ThreadID: "t", HistoryID: "h", Subject: "s",
		Messages: []store.MessageSnapshot{msg("m1", at, "INBOX"), msg("m2", at, "INBOX")}})
	row, err := s.GetThread(context.Background(), a.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, "snippet m2", row.Snippet)
}

func TestApplyThreadLeftInboxTransitions(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	at := clk.Now()
	leftAt := func() (int64, bool) {
		var v *int64
		require.NoError(t, s.raw.QueryRow(`SELECT left_inbox_at FROM threads WHERE account_id = ? AND id = 't'`, a.ID).Scan(&v))
		if v == nil {
			return 0, false
		}
		return *v, true
	}

	apply(t, s, snap(a.ID, "t", at, "INBOX"))
	_, has := leftAt()
	assert.False(t, has, "受信箱内は NULL")

	// 1→0: 遷移時刻を記録する。
	clk.Advance(time.Hour)
	tLeft := clk.Now().UnixMilli()
	apply(t, s, snap(a.ID, "t", at))
	v, has := leftAt()
	require.True(t, has)
	assert.Equal(t, tLeft, v)
	row, err := s.GetThread(ctx, a.ID, "t")
	require.NoError(t, err)
	assert.False(t, row.InInbox)

	// 0→0: 時刻は更新されない。
	clk.Advance(time.Hour)
	apply(t, s, snap(a.ID, "t", at, "TRASH"))
	v, _ = leftAt()
	assert.Equal(t, tLeft, v)

	// 0→1: NULL に戻る。
	clk.Advance(time.Hour)
	apply(t, s, snap(a.ID, "t", at, "INBOX"))
	_, has = leftAt()
	assert.False(t, has)

	// 再び 1→0: 新しい遷移時刻。
	clk.Advance(time.Hour)
	apply(t, s, snap(a.ID, "t", at))
	v, _ = leftAt()
	assert.Equal(t, clk.Now().UnixMilli(), v)
}

func TestApplyThreadNewThreadOutsideInboxIsPurgeable(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	apply(t, s, snap(a.ID, "archived", clk.Now())) // INBOX なしで初登場

	n, err := s.PurgeThreadsLeftInboxBefore(ctx, clk.Now().Add(time.Second))
	require.NoError(t, err)
	assert.Equal(t, 1, n, "left_inbox_at が NULL のままだと永久に削除されない")
}

func TestApplyThreadErrors(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")

	_, err := s.ApplyThread(ctx, store.ThreadSnapshot{AccountID: a.ID, ThreadID: "t", HistoryID: "h"})
	require.Error(t, err, "メッセージ 0 件")

	_, err = s.ApplyThread(ctx, snap("no-such-account", "t", time.Now(), "INBOX"))
	assert.ErrorIs(t, err, store.ErrNotFound)

	// 途中で失敗したらロールバックされる（重複メッセージ ID）。
	bad := snap(a.ID, "t2", time.Now(), "INBOX")
	bad.Messages = append(bad.Messages, bad.Messages[0])
	_, err = s.ApplyThread(ctx, bad)
	require.Error(t, err)
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM threads`))
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM messages`))
}

func TestReplaceInbox(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	b := seedAccount(t, s, "b@example.com")
	at := clk.Now().Add(-24 * time.Hour)

	apply(t, s, snap(a.ID, "keep", at, "INBOX"))
	apply(t, s, snap(a.ID, "gone", at, "INBOX", "UNREAD"))
	apply(t, s, snap(a.ID, "archived", at))
	apply(t, s, snap(b.ID, "other", at, "INBOX"))
	require.NoError(t, s.AddReceivedContacts(ctx, a.ID, []model.Address{{Email: "p@example.com"}}, at))
	require.NoError(t, s.ReplaceLabels(ctx, a.ID, []store.Label{{ID: "INBOX", Name: "INBOX", Type: "system"}}))

	clk.Advance(time.Hour)
	updated := snap(a.ID, "keep", at, "INBOX", "UNREAD")
	updated.HistoryID = "h2"
	err := s.ReplaceInbox(ctx, a.ID, []store.ThreadSnapshot{updated, snap(a.ID, "new", at, "INBOX")})
	require.NoError(t, err)

	keep, err := s.GetThread(ctx, a.ID, "keep")
	require.NoError(t, err)
	assert.True(t, keep.InInbox)
	assert.True(t, keep.IsUnread, "snapshots の内容で更新される")
	newRow, err := s.GetThread(ctx, a.ID, "new")
	require.NoError(t, err)
	assert.True(t, newRow.InInbox)

	gone, err := s.GetThread(ctx, a.ID, "gone")
	require.NoError(t, err)
	assert.False(t, gone.InInbox, "snapshots に無い受信箱スレッドは外れる")
	var left int64
	require.NoError(t, s.raw.QueryRow(`SELECT left_inbox_at FROM threads WHERE account_id = ? AND id = 'gone'`, a.ID).Scan(&left))
	assert.Equal(t, clk.Now().UnixMilli(), left)
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM message_labels WHERE account_id = ? AND message_id = 'gone-m1' AND label_id = 'INBOX'`, a.ID))

	archived, err := s.GetThread(ctx, a.ID, "archived")
	require.NoError(t, err)
	assert.False(t, archived.InInbox)
	other, err := s.GetThread(ctx, b.ID, "other")
	require.NoError(t, err)
	assert.True(t, other.InInbox, "他アカウントには影響しない")

	counts, err := s.UnreadCounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{a.ID: 1}, counts)
	assert.Equal(t, 1, count(t, s, `SELECT COUNT(*) FROM contacts WHERE account_id = ?`, a.ID), "contacts は保持")
	labels, err := s.ListLabels(ctx, a.ID)
	require.NoError(t, err)
	assert.Len(t, labels, 1, "labels は保持")

	assert.Error(t, s.ReplaceInbox(ctx, a.ID, []store.ThreadSnapshot{snap(b.ID, "x", at, "INBOX")}), "他アカウントのスナップショットは拒否")
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM threads WHERE id = 'x'`))
}

func TestDeleteThreadCascades(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	apply(t, s, snap(a.ID, "t", clk.Now(), "INBOX"))
	require.NoError(t, s.DeleteThread(ctx, a.ID, "t"))
	require.NoError(t, s.DeleteThread(ctx, a.ID, "t"), "再削除は冪等")
	_, err := s.GetThread(ctx, a.ID, "t")
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM messages`))
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM message_labels`))
}

func TestPurgeThreadsLeftInboxBefore(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	base := clk.Now()
	require.NoError(t, s.AddReceivedContacts(ctx, a.ID, []model.Address{{Email: "p@example.com"}}, base))

	// old: 100 日前に受信箱を外れた / recent: 10 日前 / inbox: 受信箱内。
	apply(t, s, snap(a.ID, "old", base, "INBOX"))
	apply(t, s, snap(a.ID, "recent", base, "INBOX"))
	apply(t, s, snap(a.ID, "inbox", base, "INBOX"))
	clk.Advance(-100 * 24 * time.Hour)
	apply(t, s, snap(a.ID, "old", base))
	clk.Advance(90 * 24 * time.Hour)
	apply(t, s, snap(a.ID, "recent", base))
	clk.Advance(10 * 24 * time.Hour) // 現在に戻る

	n, err := s.PurgeThreadsLeftInboxBefore(ctx, clk.Now().Add(-90*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	_, err = s.GetThread(ctx, a.ID, "old")
	assert.ErrorIs(t, err, store.ErrNotFound)
	for _, id := range []string{"recent", "inbox"} {
		_, err = s.GetThread(ctx, a.ID, id)
		assert.NoError(t, err, id)
	}
	assert.Equal(t, 2, count(t, s, `SELECT COUNT(*) FROM messages`), "messages もカスケード削除")
	assert.Equal(t, 1, count(t, s, `SELECT COUNT(*) FROM message_labels`), "inbox の INBOX のみ")
	assert.Equal(t, 1, count(t, s, `SELECT COUNT(*) FROM contacts`), "contacts は残る")

	n, err = s.PurgeThreadsLeftInboxBefore(ctx, clk.Now().Add(-90*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}
