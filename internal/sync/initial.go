package syncer

import (
	"context"
	"fmt"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

const labelInbox = "INBOX"

// inboxPage は INBOX の最新ページと、その時点の historyId。
type inboxPage struct {
	historyID string
	nextToken string
	threads   []fetched
}

// loadInbox は現在の historyId を先に取り、INBOX の最新 initialPageSize スレッドのメタデータを取得する。
// historyId を先に取ることで、取得中に起きた変更を次回の差分で拾える。
func (s *Scheduler) loadInbox(ctx context.Context, accountID string) (inboxPage, error) {
	prof, err := s.gmail.Profile(ctx, accountID)
	if err != nil {
		return inboxPage{}, err
	}
	list, err := s.gmail.ListThreads(ctx, accountID, gmail.ListQuery{
		LabelIDs:   []string{labelInbox},
		MaxResults: initialPageSize,
	})
	if err != nil {
		return inboxPage{}, err
	}
	ids := make([]string, 0, len(list.Threads))
	for _, t := range list.Threads {
		ids = append(ids, t.ID)
	}
	fs, err := s.fetchThreads(ctx, accountID, ids)
	if err != nil {
		return inboxPage{}, err
	}
	return inboxPage{historyID: prof.HistoryID, nextToken: list.NextPageToken, threads: fs}, nil
}

// initialSync は初回同期。INBOX の最新を保存し、historyId とページトークンを記録する。通知しない。
func (s *Scheduler) initialSync(ctx context.Context, cyc *cycle, st store.SyncState) (store.SyncState, error) {
	accountID := cyc.acct.ID
	page, err := s.loadInbox(ctx, accountID)
	if err != nil {
		return st, err
	}
	if err := s.refreshLabels(ctx, accountID); err != nil {
		return st, err
	}
	if err := s.ingest(ctx, cyc, page.threads); err != nil {
		return st, err
	}
	st.HistoryID = page.historyID
	st.InboxPageToken = page.nextToken
	st.InboxExhausted = page.nextToken == ""
	st.InitialSyncDone = true
	return st, nil
}

// resetSync は historyId 失効時のリセット同期。INBOX を取り直して置き換える。通知しない。
// contacts・labels は保持する（未知のラベルが現れたときだけ更新）。
func (s *Scheduler) resetSync(ctx context.Context, cyc *cycle, st store.SyncState) (store.SyncState, error) {
	accountID := cyc.acct.ID
	page, err := s.loadInbox(ctx, accountID)
	if err != nil {
		return st, err
	}
	snaps := snapshots(accountID, page.threads)
	if err := s.ensureLabels(ctx, accountID, snaps); err != nil {
		return st, err
	}
	if err := s.store.ReplaceInbox(ctx, accountID, snaps); err != nil {
		return st, fmt.Errorf("syncer: replace inbox: %w", err)
	}
	for _, snap := range snaps {
		cyc.markChanged(snap.ThreadID)
	}
	st.HistoryID = page.historyID
	st.InboxPageToken = page.nextToken
	st.InboxExhausted = page.nextToken == ""
	return st, nil
}
