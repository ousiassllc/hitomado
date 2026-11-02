package syncer

import (
	"context"
	"slices"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/notify"
	"github.com/ousiassllc/hitomado/internal/store"
)

const labelUnread = "UNREAD"

// deltaSync は history.list の差分を反映する。途中で失敗したら history_id は進めない（呼び出し側が保存しない）。
func (s *Scheduler) deltaSync(ctx context.Context, cyc *cycle, st store.SyncState) (store.SyncState, error) {
	accountID := cyc.acct.ID
	hp, err := s.gmail.ListHistory(ctx, accountID, st.HistoryID)
	if err != nil {
		return st, err
	}
	fs, err := s.fetchThreads(ctx, accountID, changedThreads(hp))
	if err != nil {
		return st, err
	}
	if err := s.ingest(ctx, cyc, fs); err != nil {
		return st, err
	}
	if cyc.acct.NotifyEnabled {
		s.detectNew(ctx, cyc, hp.Added)
	}
	if hp.LatestHistoryID != "" {
		st.HistoryID = hp.LatestHistoryID
	}
	return st, nil
}

// changedThreads は変更スレッド ID（履歴の順・重複なし）。messagesAdded のスレッドが漏れていれば補う。
func changedThreads(hp gmail.HistoryPage) []string {
	ids := slices.Clone(hp.ChangedThreadIDs)
	for _, a := range hp.Added {
		if !slices.Contains(ids, a.ThreadID) {
			ids = append(ids, a.ThreadID)
		}
	}
	return dedupe(ids)
}

// detectNew は新着通知を cyc.notices に集める。messagesAdded かつ INBOX＋UNREAD かつ新規メッセージのスレッドを
// 1 スレッド 1 通知にする。差出人・件名は DB 行から取る。
func (s *Scheduler) detectNew(ctx context.Context, cyc *cycle, added []gmail.HistoryAdded) {
	notified := map[string]struct{}{}
	for _, ad := range added {
		if _, done := notified[ad.ThreadID]; done {
			continue
		}
		if !slices.Contains(ad.LabelIDs, labelInbox) || !slices.Contains(ad.LabelIDs, labelUnread) {
			continue
		}
		if !cyc.isNew(ad.ThreadID, ad.MessageID) {
			continue
		}
		row, err := s.store.GetThread(ctx, cyc.acct.ID, ad.ThreadID)
		if err != nil {
			s.log.Warn("get thread for notice failed", "account", cyc.acct.ID, "err", err)
			continue
		}
		notified[ad.ThreadID] = struct{}{}
		from := row.LastFrom.Name
		if from == "" {
			from = row.LastFrom.Email
		}
		name := cyc.acct.DisplayName
		if name == "" {
			name = cyc.acct.Email
		}
		cyc.notices = append(cyc.notices, notify.NewMailNotice{
			AccountID:   cyc.acct.ID,
			AccountName: name,
			ThreadID:    ad.ThreadID,
			FromName:    from,
			Subject:     row.Subject,
		})
	}
}
