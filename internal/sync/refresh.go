package syncer

import (
	"context"
	"fmt"
	"time"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
)

// refreshCall はアカウント 1 件分の手動更新。実行中の呼び出しは共有し、完了から 5 秒間は結果を使い回す。
type refreshCall struct {
	done     chan struct{}
	err      error // done のクローズ後に読む
	finished bool  // s.mu で保護
	at       time.Time
}

// errNeedsReauth は再接続待ちのアカウントに対する手動更新のエラー。
func errNeedsReauth(accountID string) error {
	return fmt.Errorf("syncer: account %s: %w", accountID, gmail.ErrNeedsReauth)
}

// Refresh は差分同期を今すぐ実行して完了を待つ。アカウントは並列に実行する。
// 同じアカウントの直近の手動更新から 5 秒未満なら、その結果を返す。
// 同期は ctx のキャンセルでは中断しない（呼び出し側は待つのをやめるだけ。他の呼び出しと結果を共有するため）。
func (s *Scheduler) Refresh(ctx context.Context, accountIDs []string) []RefreshResult {
	ids := accountIDs
	if len(ids) == 0 {
		accounts, err := s.store.ListAccounts(ctx)
		if err != nil {
			s.log.Warn("refresh: list accounts failed", "err", err)
			return []RefreshResult{}
		}
		for _, a := range accounts {
			ids = append(ids, a.ID)
		}
	}
	ids = dedupe(ids)
	calls := make([]*refreshCall, len(ids))
	for i, id := range ids {
		calls[i] = s.refreshCall(id)
	}
	results := make([]RefreshResult, len(ids))
	for i, c := range calls {
		results[i].AccountID = ids[i]
		select {
		case <-c.done:
			results[i].Err = c.err
		case <-ctx.Done():
			results[i].Err = ctx.Err()
		}
	}
	return results
}

// refreshCall は id の手動更新を開始する。実行中または 5 秒以内に完了済みのものがあればそれを返す。
func (s *Scheduler) refreshCall(id string) *refreshCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return &refreshCall{done: closedChan(), err: ErrClosed, finished: true}
	}
	now := s.clock.Now()
	if c := s.refreshes[id]; c != nil && (!c.finished || now.Sub(c.at) < refreshSuppress) {
		return c
	}
	c := &refreshCall{done: make(chan struct{})}
	s.refreshes[id] = c
	a := s.accountLocked(id)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := s.runRefreshCycle(a)
		s.mu.Lock()
		c.err, c.at, c.finished = err, s.clock.Now(), true
		s.mu.Unlock()
		close(c.done)
	}()
	return c
}

// runRefreshCycle は手動更新の 1 周期を実行する。panic は記録してエラーにする
// （呼び出し側が c.err を設定して c.done を閉じられるようにし、Refresh を戻らなくしない）。
func (s *Scheduler) runRefreshCycle(a *account) (err error) {
	defer logging.RecoverTo(s.log, "sync refresh", &err)
	return s.runCycle(a.ctx, a)
}

func closedChan() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

func dedupe(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
