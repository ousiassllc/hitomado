package oauth

import (
	"context"
	"errors"
	"log/slog"
	"regexp"

	"github.com/ousiassllc/hitomado/internal/logging"
)

// oauthErrorRE は OAuth の error コード（RFC 6749 §5.2。invalid_grant など）として記録してよい形。
// 応答・コールバックの値をそのまま載せないよう、形式を検査した短い識別子だけを通す。
var oauthErrorRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// safeOAuthError は v が OAuth の error コードの形ならそのまま、そうでなければ空を返す。
func safeOAuthError(v string) string {
	if oauthErrorRE.MatchString(v) {
		return v
	}
	return ""
}

// logProgress は認可の段階の進み（待受の開始・ブラウザへの受け渡し・ブラウザからの戻り・完了）を Info で記録する。
// 失敗したときにどの段階まで進んだかを、ログから追えるようにする。値（コード・state・トークン）は載せない。
func (f *flow) logProgress(stage string) {
	f.opts.Logger.Info("oauth: "+stage, "stage", stage)
}

// logFailure は認可の段階 stage での失敗を記録する。Authorize の呼び出し元が INTERNAL などに丸める前の、元のエラーを残す。
// 利用者の操作（キャンセル・拒否・時間切れ）は Warn、それ以外は Error。
// HTTP の失敗ならステータスと OAuth の error コードを付ける（応答本文・error_description は載せない）。
func (f *flow) logFailure(stage string, err error, extra ...any) {
	level := slog.LevelError
	switch {
	case errors.Is(err, ErrCancelled), errors.Is(err, ErrDenied), errors.Is(err, ErrTimeout),
		errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		level = slog.LevelWarn
	}
	attrs := append([]any{"stage", stage, "err", err, "chain", logging.ErrorChain(err)}, extra...)
	var he *httpError
	if errors.As(err, &he) {
		attrs = append(attrs, "status", he.status)
		if he.code != "" {
			attrs = append(attrs, "oauth_error", he.code)
		}
	}
	f.opts.Logger.Log(context.Background(), level, "oauth: failed at "+stage, attrs...)
}
