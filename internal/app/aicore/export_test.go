package aicore

import "time"

// SetTimeoutForTest は生成のタイムアウトを短縮する（テスト専用）。
func (s *Service) SetTimeoutForTest(d time.Duration) { s.timeout = d }
