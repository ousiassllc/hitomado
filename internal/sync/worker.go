package syncer

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/logging"
)

func (a *account) signal() {
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

// runWorker はアカウントの周期同期ループ。needs_reauth の間は周期・要求を無視する。
func (s *Scheduler) runWorker(a *account) {
	defer s.wg.Done()
	defer close(a.done)
	tick, stop := s.newTicker(s.interval)
	defer stop()
	s.cycle(a)
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-tick:
			s.cycle(a)
		case <-a.kick:
			s.cycle(a)
		}
	}
}

func (s *Scheduler) cycle(a *account) {
	defer logging.Recover(s.log, "sync cycle") // 1 周期の panic でワーカーを止めない（スタックを記録して次の周期へ）
	if a.paused.Load() {
		return
	}
	if err := s.runCycle(a.ctx, a); err != nil && a.ctx.Err() == nil {
		s.log.Warn("sync cycle failed", "account", a.id, "err", err)
	}
}

// runCycle は 1 周期の同期を直列化して実行する。
func (s *Scheduler) runCycle(ctx context.Context, a *account) error {
	a.syncMu.Lock()
	defer a.syncMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.paused.Load() {
		return errNeedsReauth(a.id)
	}
	return s.syncAccount(ctx, a)
}
