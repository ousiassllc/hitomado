package gmail

import (
	"context"
	"errors"
	"log/slog"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/ousiassllc/nefos-inbox/internal/gmail/backoff"
)

// maxRetries は 429／5xx の再試行回数の上限（初回を除く）。
const maxRetries = 6

// クォータユニット（Gmail API のクォータ計算。ユーザーあたり 250 units/秒）。
const (
	unitsLabels      = 1
	unitsSettings    = 1
	unitsHistory     = 2
	unitsAttachment  = 5
	unitsMessageList = 5
	unitsMessageGet  = 5
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
	// transfer が真なら、総時間の上限を持たない転送用サービスで呼ぶ（添付・生 MIME の大きな送受信）。
	// 打ち切りは呼び出し側 ctx の期限と応答ヘッダー待ちに任せる。
	transfer bool
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
		if attempt == 0 {
			// 同じアカウントのほかの呼び出しが利用上限で待っている間は、新しい呼び出しも待つ（上限中に投げ足して延ばさない）。
			if d := a.cool.Remaining(); d > 0 {
				if err := c.sleep(ctx, d); err != nil {
					return ctxError(ctx, spec.name)
				}
			}
		}
		if err := a.limiter.WaitN(ctx, spec.units); err != nil {
			return ctxError(ctx, spec.name)
		}
		svc := a.svc
		if spec.transfer {
			svc = a.xfer
		}
		err := op(svc)
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
			// 再接続が必要になる前触れ。2 回目も 401 なら ErrNeedsReauth（reason=api_401）になるので、1 回目も残して頻度を追えるようにする。
			c.logger.Info("Gmail API が 401 を返しました（アクセストークンを取り直して再試行）",
				slog.String("account_id", accountID), slog.String("op", spec.name))
			a.auth.Invalidate()
			continue
		case errors.Is(f.kind, ErrNeedsReauth):
			c.Forget(accountID) // 再接続で保存し直されたトークンを次回読み直す
		case f.retry == retryLimited || (f.retry == retryServer && spec.retry5xx):
			if attempt < maxRetries {
				wait := c.jitter(backoff.Delay(attempt))
				if f.retry == retryLimited {
					// 上限は分単位の枠なので、1 秒起点では枠が戻る前に再試行を使い切る。4 秒起点（4・8・16・32・60・60 秒）で、
					// 待ちの下限を半分にして（短すぎる待ちを避ける）、合計で 1 分を超えるようにする。
					d := backoff.Delay(attempt + 2)
					wait = d/2 + c.jitter(d/2)
					// 並列の呼び出しが別々に待って同時に投げ直すと、また当たる。アカウントで待ちを共有する。
					wait = max(wait, a.cool.Remaining())
					a.cool.Extend(wait)
				}
				c.logger.Debug("Gmail API を再試行", slog.String("account_id", accountID),
					slog.String("op", spec.name), slog.Int("attempt", attempt+1), slog.Int("status", f.status))
				if err := c.sleep(ctx, wait); err != nil {
					return ctxError(ctx, spec.name)
				}
				continue
			}
		}
		wrapped := wrapFailure(spec.name, f)
		c.logAPIFailure(accountID, wrapped)
		return wrapped
	}
}

// logAPIFailure は Gmail API がエラー応答を返して失敗した呼び出しを、Google が返した reason・status・message つきで記録する。
// 値は APIError の構築時に伏せ字処理・文字種の制限を済ませている。存在確認などで想定内の 404 は Debug に留める。
func (c *client) logAPIFailure(accountID string, err error) {
	ae, ok := AsAPIError(err)
	if !ok {
		return
	}
	level := slog.LevelWarn
	if errors.Is(err, ErrNotFound) {
		level = slog.LevelDebug
	}
	c.logger.Log(context.Background(), level, "Gmail API がエラーを返しました", append([]any{slog.String("account_id", accountID)}, ae.logAttrs()...)...)
}
