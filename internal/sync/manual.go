package syncer

import (
	"context"
	"errors"
	"fmt"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// account は id の実行時状態を返す（無ければ作る）。Shutdown 済みなら ErrClosed。
func (s *Scheduler) account(id string) (*account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	return s.accountLocked(id), nil
}

// FetchMore は inbox_page_token で受信箱の続きを取得して保存する。末尾に達したら inbox_exhausted を立てる。
// added は新規メッセージを含んでいた（＝新たに保存した）スレッド数。
func (s *Scheduler) FetchMore(ctx context.Context, accountID string) (int, error) {
	a, err := s.account(accountID)
	if err != nil {
		return 0, err
	}
	ctx, cancel := joinCtx(ctx, a)
	defer cancel()
	a.syncMu.Lock()
	defer a.syncMu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	added, err := s.fetchMore(ctx, accountID)
	if err != nil {
		s.noteReauth(ctx, a, err)
	}
	return added, err
}

func (s *Scheduler) fetchMore(ctx context.Context, accountID string) (int, error) {
	acct, err := s.store.GetAccount(ctx, accountID)
	if err != nil {
		return 0, fmt.Errorf("syncer: get account: %w", err)
	}
	st, err := s.store.GetSyncState(ctx, accountID)
	if err != nil {
		return 0, fmt.Errorf("syncer: get sync state: %w", err)
	}
	if st.InboxExhausted || !st.InitialSyncDone {
		return 0, nil
	}
	if st.InboxPageToken == "" {
		// 初回同期後にトークンが無いのは末尾に達した状態。先頭ページを取り直さない。
		st.InboxExhausted = true
		return 0, s.saveState(ctx, st)
	}
	list, err := s.gmail.ListThreads(ctx, accountID, gmail.ListQuery{
		LabelIDs:   []string{labelInbox},
		PageToken:  st.InboxPageToken,
		MaxResults: morePageSize,
	})
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(list.Threads))
	for _, t := range list.Threads {
		ids = append(ids, t.ID)
	}
	fs, err := s.fetchThreads(ctx, accountID, ids)
	if err != nil {
		return 0, err
	}
	cyc := newCycle(acct)
	ingestErr := s.ingest(ctx, cyc, fs)
	// 反映済みの連絡先・未読表示は失敗時も出す。ページトークンは成功時のみ進める。
	s.flushQuiet(ctx, cyc)
	if ingestErr != nil {
		return 0, ingestErr
	}
	st.InboxPageToken = list.NextPageToken
	st.InboxExhausted = list.NextPageToken == ""
	if err := s.saveState(ctx, st); err != nil {
		return 0, err
	}
	return len(cyc.newMsgs), nil
}

func (s *Scheduler) saveState(ctx context.Context, st store.SyncState) error {
	if err := s.store.SaveSyncState(ctx, st); err != nil {
		return fmt.Errorf("syncer: save sync state: %w", err)
	}
	return nil
}

// RefreshThread は当該スレッドのメタデータを再取得して DB に反映する。
// Gmail 側に存在しなければ DB からも削除し found=false を返す。
func (s *Scheduler) RefreshThread(ctx context.Context, accountID, threadID string) (store.ThreadRow, bool, error) {
	a, err := s.account(accountID)
	if err != nil {
		return store.ThreadRow{}, false, err
	}
	ctx, cancel := joinCtx(ctx, a)
	defer cancel()
	row, found, err := s.refreshThread(ctx, accountID, threadID)
	if err != nil {
		s.noteReauth(ctx, a, err)
	}
	return row, found, err
}

func (s *Scheduler) refreshThread(ctx context.Context, accountID, threadID string) (store.ThreadRow, bool, error) {
	fs, err := s.fetchThreads(ctx, accountID, []string{threadID})
	if err != nil {
		return store.ThreadRow{}, false, err
	}
	cyc := newCycle(store.Account{ID: accountID})
	if err := s.ingest(ctx, cyc, fs); err != nil {
		return store.ThreadRow{}, false, err
	}
	s.flushQuiet(ctx, cyc)
	if fs[0].gone {
		return store.ThreadRow{}, false, nil
	}
	row, err := s.store.GetThread(ctx, accountID, threadID)
	if err != nil {
		return store.ThreadRow{}, false, fmt.Errorf("syncer: get thread: %w", err)
	}
	return row, true, nil
}

// flushQuiet は利用者操作・ページ取得の後始末。連絡先とトレイの未読表示を更新する（sync:updated は出さない）。
func (s *Scheduler) flushQuiet(ctx context.Context, cyc *cycle) {
	cyc.changed = nil
	s.flush(ctx, cyc)
}

// noteReauth は手動操作で ErrNeedsReauth／ErrClientRejected を受けたとき、アカウントを needs_reauth にして自動再試行を止める。
func (s *Scheduler) noteReauth(ctx context.Context, a *account, err error) {
	if (!errors.Is(err, gmail.ErrNeedsReauth) && !errors.Is(err, gmail.ErrClientRejected)) || ctx.Err() != nil {
		return
	}
	status, msg := classify(err)
	a.paused.Store(true)
	cur, getErr := s.store.GetAccount(ctx, a.id)
	if getErr != nil {
		s.log.Warn("get account failed", "account", a.id, "err", getErr)
		return
	}
	s.publishStatus(ctx, a, cur, status, msg)
}

// waitIdle は実行中の同期周期・FetchMore の終了を待つ。
func (a *account) waitIdle() {
	a.syncMu.Lock()
	defer a.syncMu.Unlock()
}
