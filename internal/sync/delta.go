package syncer

import (
	"context"
	"slices"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/notify"
	"github.com/ousiassllc/hitomado/internal/store"
)

const (
	labelUnread = "UNREAD"
	labelDraft  = "DRAFT"
)

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
	if cyc.acct.NotifyEnabled {
		cyc.candidates = candidatesOf(hp.Added)
	}
	if cyc.observe = s.observer.Load() != nil; cyc.observe {
		cyc.observedAdded = make(map[string]struct{}, len(hp.Added))
		for _, ad := range hp.Added {
			cyc.observedAdded[ad.MessageID] = struct{}{}
		}
	}
	if err := s.ingest(ctx, cyc, fs); err != nil {
		return st, err
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

// candidatesOf は通知候補（messagesAdded かつ INBOX＋UNREAD。DRAFT は除く）をメッセージ ID で引ける形にする。
func candidatesOf(added []gmail.HistoryAdded) map[string]gmail.HistoryAdded {
	out := map[string]gmail.HistoryAdded{}
	for _, ad := range added {
		if slices.Contains(ad.LabelIDs, labelInbox) && slices.Contains(ad.LabelIDs, labelUnread) &&
			!slices.Contains(ad.LabelIDs, labelDraft) {
			out[ad.MessageID] = ad
		}
	}
	return out
}

// collectNotice は ApplyThread 直後に呼び、新規メッセージが通知候補なら通知を集める。
// スレッドを反映するたびに集めるので、後続のスレッドで周期が失敗しても通知は失われない。
// 1 スレッド 1 通知にし、差出人・件名は DB 行から取る。
func (s *Scheduler) collectNotice(ctx context.Context, cyc *cycle, threadID string, newIDs []string) {
	if _, done := cyc.notified[threadID]; done {
		return
	}
	if !slices.ContainsFunc(newIDs, func(id string) bool { _, ok := cyc.candidates[id]; return ok }) {
		return
	}
	row, err := s.store.GetThread(ctx, cyc.acct.ID, threadID)
	if err != nil {
		s.log.Warn("get thread for notice failed", "account", cyc.acct.ID)
		return
	}
	cyc.notified[threadID] = struct{}{}
	from := row.LastFrom.Name
	if from == "" {
		from = row.LastFrom.Email
	}
	name := cyc.acct.DisplayName
	if name == "" {
		name = cyc.acct.Email
	}
	cyc.notices = append(cyc.notices, notify.NewMailNotice{
		AccountID:    cyc.acct.ID,
		AccountName:  name,
		AccountEmail: cyc.acct.Email,
		ThreadID:     threadID,
		FromName:     from,
		Subject:      row.Subject,
	})
}
