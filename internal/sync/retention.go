package syncer

import "github.com/ousiassllc/hitomado/internal/logging"

// runRetention は保持期間を過ぎたスレッドを起動直後と 24 時間ごとに削除する。
func (s *Scheduler) runRetention() {
	defer s.wg.Done()
	tick, stop := s.newTicker(retentionEvery)
	defer stop()
	s.purge()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-tick:
			s.purge()
		}
	}
}

func (s *Scheduler) purge() {
	defer logging.Recover(s.log, "retention purge") // panic で毎日の掃除ループ（とアプリ）を止めない
	n, err := s.store.PurgeThreadsLeftInboxBefore(s.ctx, s.clock.Now().Add(-retentionPeriod))
	if err != nil {
		if s.ctx.Err() == nil {
			s.log.Warn("retention purge failed", "err", err)
		}
		return
	}
	if n > 0 {
		s.log.Info("retention purge", "deleted", n)
	}
}
