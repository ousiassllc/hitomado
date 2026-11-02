package gmail

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
)

const (
	backoffBase = time.Second
	backoffMax  = 60 * time.Second
	// maxRetries は 429／5xx の再試行回数の上限（初回を除く）。
	maxRetries = 6
)

// クォータユニット（Gmail API のクォータ計算。ユーザーあたり 250 units/秒）。
const (
	unitsLabels      = 1
	unitsHistory     = 2
	unitsAttachment  = 5
	unitsThread      = 10
	unitsDraft       = 10
	unitsMessageSend = 100
)

// callSpec は 1 回の API 呼び出しの種別。
type callSpec struct {
	name  string // ログ・エラー文言用の操作名
	units int    // 消費するクォータユニット
	// retry5xx が偽なら 5xx を再試行しない（messages.send など、再送で二重実行になり得る呼び出し）。
	retry5xx bool
}

// do はアカウントのレート制限を通して op を実行し、429／5xx を指数バックオフで再試行する。
// 失敗は公開 sentinel を %w で包んだエラーにして返す。ctx のキャンセルでは待ちを即中断する。
func (c *client) do(ctx context.Context, accountID string, spec callSpec, op func(s *gmailapi.Service) error) error {
	a, err := c.account(ctx, accountID)
	if err != nil {
		return err
	}
	authRetried := false
	for attempt := 0; ; attempt++ {
		if err := a.limiter.WaitN(ctx, spec.units); err != nil {
			return ctxError(ctx, spec.name)
		}
		err := op(a.svc)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctxError(ctx, spec.name)
		}
		f := classify(err)
		switch {
		case f.retry == retryAuth && !authRetried:
			// アクセストークンが無効化された可能性。更新して 1 回だけやり直す。
			authRetried = true
			a.auth.Invalidate()
			continue
		case errors.Is(f.kind, ErrNeedsReauth):
			c.Forget(accountID) // 再接続で保存し直されたトークンを次回読み直す
		case f.retry == retryLimited || (f.retry == retryServer && spec.retry5xx):
			if attempt < maxRetries {
				wait := c.jitter(backoffDelay(attempt))
				c.logger.Debug("Gmail API を再試行", slog.String("account_id", accountID),
					slog.String("op", spec.name), slog.Int("attempt", attempt+1), slog.Int("status", f.status))
				if err := c.sleep(ctx, wait); err != nil {
					return ctxError(ctx, spec.name)
				}
				continue
			}
		}
		return wrapFailure(spec.name, f)
	}
}

// backoffDelay は attempt 回目（0 始まり）の再試行前の待ち時間の上限（1 秒から倍々、最大 60 秒）。
func backoffDelay(attempt int) time.Duration {
	d := backoffBase << attempt
	if d <= 0 || d > backoffMax {
		return backoffMax
	}
	return d
}

// fullJitter は [0, d] の一様乱数（フルジッタ）。
func fullJitter(d time.Duration) time.Duration {
	return time.Duration(rand.Int64N(int64(d) + 1)) //nolint:gosec // バックオフのジッタに暗号論的乱数は不要
}

// sleepCtx は d だけ待つ。ctx がキャンセルされたら即座にそのエラーを返す。
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
