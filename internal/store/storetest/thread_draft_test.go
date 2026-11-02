package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/store"
)

func TestApplyThreadExcludesDrafts(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	// 最新の DRAFT（INBOX・UNREAD 付き）は保存せず、最新メッセージ・件数・未読・受信箱の集計にも使わない。
	res := apply(t, s, store.ThreadSnapshot{
		AccountID: a.ID, ThreadID: "t", HistoryID: "h", Subject: "s",
		Messages: []store.MessageSnapshot{
			msg("m1", t0, "SENT"),
			msg("d1", t0.Add(time.Hour), "DRAFT", "INBOX", "UNREAD"),
		},
	})
	assert.Equal(t, []string{"m1"}, res.NewMessageIDs)
	row, err := s.GetThread(ctx, a.ID, "t")
	require.NoError(t, err)
	assert.Equal(t, 1, row.MessageCount)
	assert.Equal(t, "snippet m1", row.Snippet)
	assert.Equal(t, "from-m1@example.com", row.LastFrom.Email)
	assert.Equal(t, t0, row.LastMessageAt)
	assert.False(t, row.IsUnread)
	assert.False(t, row.InInbox)
	assert.Equal(t, 1, count(t, s, `SELECT COUNT(*) FROM messages`))
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM messages WHERE id = 'd1'`))
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM message_labels WHERE label_id = 'DRAFT'`))

	// 下書きだけのスレッドは保存されず、既存の行があれば削除される。
	only := store.ThreadSnapshot{AccountID: a.ID, ThreadID: "t", HistoryID: "h2", Subject: "s",
		Messages: []store.MessageSnapshot{msg("d2", t0, "DRAFT")}}
	res = apply(t, s, only)
	assert.Empty(t, res.NewMessageIDs)
	assert.NotNil(t, res.NewMessageIDs)
	_, err = s.GetThread(ctx, a.ID, "t")
	assert.ErrorIs(t, err, store.ErrNotFound)
	only.ThreadID = "never-saved"
	apply(t, s, only)
	_, err = s.GetThread(ctx, a.ID, "never-saved")
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM messages`))

	// ReplaceInbox でも同様（下書きだけのスレッドは保存されない）。
	require.NoError(t, s.ReplaceInbox(ctx, a.ID, []store.ThreadSnapshot{only}))
	assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM threads`))
}
