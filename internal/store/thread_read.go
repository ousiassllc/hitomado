package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store/db"
)

// defaultInboxLimit は InboxFilter.Limit が 0 以下のときの取得件数。
const defaultInboxLimit = 50

func (s *sqlStore) DeleteThread(ctx context.Context, accountID, threadID string) error {
	// 対象が無いときも成功とする（既に削除済みの Gmail スレッドの同期を冪等にするため）。
	if _, err := s.q.DeleteThread(ctx, db.DeleteThreadParams{AccountID: accountID, ID: threadID}); err != nil {
		return fmt.Errorf("store: delete thread: %w", err)
	}
	return nil
}

func (s *sqlStore) GetThread(ctx context.Context, accountID, threadID string) (ThreadRow, error) {
	r, err := s.q.GetThread(ctx, db.GetThreadParams{AccountID: accountID, ID: threadID})
	if err != nil {
		return ThreadRow{}, fmt.Errorf("store: get thread: %w", notFoundIfNoRows(err))
	}
	row := threadRowFromRow(r)
	row.Labels, err = s.threadLabels(ctx, accountID, threadID)
	return row, err
}

func threadRowFromRow(r db.GetThreadRow) ThreadRow {
	return ThreadRow{
		AccountID:     r.AccountID,
		ThreadID:      r.ID,
		Subject:       r.Subject,
		Snippet:       r.Snippet,
		LastMessageAt: fromMillis(r.LastMessageAt),
		LastFrom:      model.Address{Name: r.LastFromName.String, Email: r.LastFromEmail},
		MessageCount:  int(r.MessageCount),
		IsUnread:      r.IsUnread == 1,
		HasAttachment: r.HasAttachment == 1,
		InInbox:       r.InInbox == 1,
		Labels:        []Label{},
	}
}

func (s *sqlStore) threadLabels(ctx context.Context, accountID, threadID string) ([]Label, error) {
	rows, err := s.q.ListThreadUserLabels(ctx, db.ListThreadUserLabelsParams{AccountID: accountID, ThreadID: threadID})
	if err != nil {
		return nil, fmt.Errorf("store: thread labels: %w", err)
	}
	out := make([]Label, 0, len(rows))
	for _, r := range rows {
		out = append(out, Label{ID: r.ID, Name: r.Name, Type: r.Type, ColorBg: r.ColorBg.String, ColorFg: r.ColorFg.String})
	}
	return out, nil
}

// ListInbox は統合受信箱（in_inbox=1）を (last_message_at DESC, account_id, id) 順で返す。
// アカウント絞り込みの IN 句が動的なため、sqlc を使わず手書きの SQL にしている。
func (s *sqlStore) ListInbox(ctx context.Context, f InboxFilter) ([]ThreadRow, error) {
	query, args := inboxQuery(f)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list inbox: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []ThreadRow{}
	for rows.Next() {
		var r db.GetThreadRow
		if err := rows.Scan(&r.AccountID, &r.ID, &r.Subject, &r.Snippet, &r.LastMessageAt, &r.LastFromName,
			&r.LastFromEmail, &r.MessageCount, &r.IsUnread, &r.HasAttachment, &r.InInbox); err != nil {
			return nil, fmt.Errorf("store: list inbox: %w", err)
		}
		out = append(out, threadRowFromRow(r))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list inbox: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("store: list inbox: %w", err)
	}
	for i := range out {
		if out[i].Labels, err = s.threadLabels(ctx, out[i].AccountID, out[i].ThreadID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// inboxQuery は ListInbox の SQL と引数を組み立てる（値はすべてプレースホルダ）。
func inboxQuery(f InboxFilter) (string, []any) {
	var b strings.Builder
	b.WriteString(`SELECT account_id, id, subject, snippet, last_message_at, last_from_name, last_from_email,
       message_count, is_unread, has_attachment, in_inbox
FROM threads WHERE in_inbox = 1`)
	var args []any
	if len(f.AccountIDs) > 0 {
		b.WriteString(" AND account_id IN (?" + strings.Repeat(",?", len(f.AccountIDs)-1) + ")")
		for _, id := range f.AccountIDs {
			args = append(args, id)
		}
	}
	if f.UnreadOnly || f.Tab == InboxTabNew {
		b.WriteString(" AND is_unread = 1")
	}
	switch f.Tab {
	case InboxTabSocial:
		b.WriteString(" AND " + threadHasLabel("= 'CATEGORY_SOCIAL'"))
	case InboxTabPromotions:
		b.WriteString(" AND " + threadHasLabel("= 'CATEGORY_PROMOTIONS'"))
	case InboxTabPrimary:
		// Gmail 本家と同じく、カテゴリを持たない受信箱のスレッドもメインに出す。
		b.WriteString(" AND (" + threadHasLabel("= 'CATEGORY_PERSONAL'") + " OR NOT " + threadHasLabel("LIKE 'CATEGORY\\_%' ESCAPE '\\'") + ")")
	}
	if c := f.After; c != nil {
		ms := toMillis(c.LastMessageAt)
		b.WriteString(` AND (last_message_at < ? OR (last_message_at = ? AND (account_id > ? OR (account_id = ? AND id > ?))))`)
		args = append(args, ms, ms, c.AccountID, c.AccountID, c.ThreadID)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = defaultInboxLimit
	}
	b.WriteString(" ORDER BY last_message_at DESC, account_id, id LIMIT ?")
	args = append(args, limit)
	return b.String(), args
}

// threadHasLabel は、外側の threads 行のいずれかのメッセージが label_id の条件（cond。固定文字列のみ）を満たす
// ラベルを持つ、という述語を返す。スレッドのラベルは in_inbox／is_unread と同じく、メッセージのラベルの和集合。
func threadHasLabel(cond string) string {
	return `EXISTS (SELECT 1 FROM messages m JOIN message_labels ml ON ml.account_id = m.account_id AND ml.message_id = m.id
 WHERE m.account_id = threads.account_id AND m.thread_id = threads.id AND ml.label_id ` + cond + `)`
}

func (s *sqlStore) UnreadCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.q.UnreadCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: unread counts: %w", err)
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.AccountID] = int(r.Unread)
	}
	return out, nil
}

// PurgeThreadsLeftInboxBefore は in_inbox=0 かつ left_inbox_at < cutoff のスレッドを削除し（messages／
// message_labels はカスケード）、削除件数を返す。contacts は削除しない。
func (s *sqlStore) PurgeThreadsLeftInboxBefore(ctx context.Context, cutoff time.Time) (int, error) {
	n, err := s.q.PurgeThreadsLeftInboxBefore(ctx, sql.NullInt64{Int64: toMillis(cutoff), Valid: true})
	if err != nil {
		return 0, fmt.Errorf("store: purge threads: %w", err)
	}
	return int(n), nil
}
