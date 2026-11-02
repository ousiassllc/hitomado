// Package backoff は再試行の待ち時間（指数バックオフ＋フルジッタ）と、ctx で中断できる待ちを提供する。
package backoff

import (
	"context"
	"math/rand/v2"
	"time"
)

const (
	// Base は 1 回目の再試行前の待ち時間の上限。
	Base = time.Second
	// Max は待ち時間の上限。
	Max = 60 * time.Second
)

// Delay は attempt 回目（0 始まり）の再試行前の待ち時間の上限（Base から倍々、最大 Max）。
func Delay(attempt int) time.Duration {
	d := Base << attempt
	if d <= 0 || d > Max {
		return Max
	}
	return d
}

// FullJitter は [0, d] の一様乱数（フルジッタ）。
func FullJitter(d time.Duration) time.Duration {
	return time.Duration(rand.Int64N(int64(d) + 1)) //nolint:gosec // バックオフのジッタに暗号論的乱数は不要
}

// Sleep は d だけ待つ。ctx がキャンセルされたら即座にそのエラーを返す。
func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
