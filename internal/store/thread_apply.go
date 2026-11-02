package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ousiassllc/hitomado/internal/store/db"
)

// Gmail のシステムラベル ID（集計に使う）。
const (
	labelInbox  = "INBOX"
	labelUnread = "UNREAD"
)

// ApplyThread は 1 スレッド分のスナップショットを 1 トランザクションで反映する。
func (s *sqlStore) ApplyThread(ctx context.Context, snap ThreadSnapshot) (ApplyResult, error) {
	var res ApplyResult
	err := s.withTx(ctx, func(q *db.Queries) error {
		ids, err := applyThread(ctx, q, snap, s.now())
		res.NewMessageIDs = ids
		return err
	})
	if err != nil {
		return ApplyResult{NewMessageIDs: []string{}}, err
	}
	return res, nil
}

// ReplaceInbox はリセット同期用。snapshots を反映し、snapshots に無い in_inbox スレッドは受信箱から外した扱い
// （in_inbox=0、left_inbox_at=現在時刻、INBOX ラベルの除去）にする。contacts・labels は触らない。
func (s *sqlStore) ReplaceInbox(ctx context.Context, accountID string, snaps []ThreadSnapshot) error {
	return s.withTx(ctx, func(q *db.Queries) error {
		now := s.now()
		keep := make(map[string]struct{}, len(snaps))
		for _, snap := range snaps {
			if snap.AccountID != accountID {
				return fmt.Errorf("store: replace inbox: snapshot belongs to another account")
			}
			if _, err := applyThread(ctx, q, snap, now); err != nil {
				return err
			}
			keep[snap.ThreadID] = struct{}{}
		}
		ids, err := q.ListInboxThreadIDs(ctx, accountID)
		if err != nil {
			return fmt.Errorf("store: replace inbox: %w", err)
		}
		for _, id := range ids {
			if _, ok := keep[id]; ok {
				continue
			}
			if err := q.DeleteThreadInboxLabels(ctx, db.DeleteThreadInboxLabelsParams{AccountID: accountID, ThreadID: id}); err != nil {
				return fmt.Errorf("store: replace inbox: %w", err)
			}
			err := q.MarkThreadLeftInbox(ctx, db.MarkThreadLeftInboxParams{
				AccountID:   accountID,
				ID:          id,
				LeftInboxAt: sql.NullInt64{Int64: toMillis(now), Valid: true},
			})
			if err != nil {
				return fmt.Errorf("store: replace inbox: %w", err)
			}
		}
		return nil
	})
}

// threadAggregate は messages／message_labels（スナップショット）から導出する threads の集計値。
type threadAggregate struct {
	latest   MessageSnapshot
	unread   bool
	inInbox  bool
	messages int
}

func aggregate(msgs []MessageSnapshot) threadAggregate {
	agg := threadAggregate{messages: len(msgs)}
	for i, m := range msgs {
		if i == 0 || !m.InternalDate.Before(agg.latest.InternalDate) {
			agg.latest = m
		}
		for _, l := range m.LabelIDs {
			switch l {
			case labelUnread:
				agg.unread = true
			case labelInbox:
				agg.inInbox = true
			}
		}
	}
	return agg
}

// applyThread は q のトランザクション内でスレッドを置き換え、新規メッセージ ID（空でも非 nil）を返す。
func applyThread(ctx context.Context, q *db.Queries, snap ThreadSnapshot, now time.Time) ([]string, error) {
	if len(snap.Messages) == 0 {
		return nil, errors.New("store: apply thread: snapshot has no messages")
	}
	agg := aggregate(snap.Messages)

	meta, err := q.GetThreadMeta(ctx, db.GetThreadMetaParams{AccountID: snap.AccountID, ID: snap.ThreadID})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: apply thread: %w", err)
	}
	// left_inbox_at: 受信箱内は NULL。外れた時刻（1→0）を記録し、外れたままなら既存値を保つ。
	// 受信箱に一度も入っていない新規スレッドや値が欠けた行は、保持期間削除の対象になるよう現在時刻を入れる。
	left := sql.NullInt64{}
	if !agg.inInbox {
		left = sql.NullInt64{Int64: toMillis(now), Valid: true}
		if err == nil && meta.InInbox == 0 && meta.LeftInboxAt.Valid {
			left = meta.LeftInboxAt
		}
	}

	oldIDs, err := q.ListThreadMessageIDs(ctx, db.ListThreadMessageIDsParams{AccountID: snap.AccountID, ThreadID: snap.ThreadID})
	if err != nil {
		return nil, fmt.Errorf("store: apply thread: %w", err)
	}
	known := make(map[string]struct{}, len(oldIDs))
	for _, id := range oldIDs {
		known[id] = struct{}{}
	}

	err = q.UpsertThread(ctx, db.UpsertThreadParams{
		AccountID:     snap.AccountID,
		ID:            snap.ThreadID,
		Subject:       snap.Subject,
		Snippet:       agg.latest.Snippet,
		LastMessageAt: toMillis(agg.latest.InternalDate),
		LastFromName:  nullString(agg.latest.FromName),
		LastFromEmail: strings.ToLower(strings.TrimSpace(agg.latest.FromEmail)),
		MessageCount:  int64(agg.messages),
		IsUnread:      boolInt(agg.unread),
		HasAttachment: boolInt(snap.HasAttachment),
		InInbox:       boolInt(agg.inInbox),
		LeftInboxAt:   left,
		HistoryID:     snap.HistoryID,
	})
	if isForeignKey(err) {
		return nil, fmt.Errorf("store: apply thread: account: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: apply thread: %w", err)
	}

	// messages／message_labels を丸ごと置き換える（message_labels はカスケード削除）。
	if err := q.DeleteThreadMessages(ctx, db.DeleteThreadMessagesParams{AccountID: snap.AccountID, ThreadID: snap.ThreadID}); err != nil {
		return nil, fmt.Errorf("store: apply thread: %w", err)
	}
	newIDs := []string{}
	for _, m := range snap.Messages {
		err := q.InsertMessage(ctx, db.InsertMessageParams{
			AccountID:    snap.AccountID,
			ID:           m.ID,
			ThreadID:     snap.ThreadID,
			InternalDate: toMillis(m.InternalDate),
			FromName:     nullString(m.FromName),
			FromEmail:    strings.ToLower(strings.TrimSpace(m.FromEmail)),
			Snippet:      m.Snippet,
		})
		if err != nil {
			return nil, fmt.Errorf("store: apply thread: insert message: %w", err)
		}
		for _, l := range m.LabelIDs {
			err := q.InsertMessageLabel(ctx, db.InsertMessageLabelParams{AccountID: snap.AccountID, MessageID: m.ID, LabelID: l})
			if err != nil {
				return nil, fmt.Errorf("store: apply thread: insert message label: %w", err)
			}
		}
		if _, ok := known[m.ID]; !ok {
			newIDs = append(newIDs, m.ID)
		}
	}
	return newIDs, nil
}
