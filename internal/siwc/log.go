package siwc

import (
	"context"
	"errors"
	"log/slog"

	"github.com/ousiassllc/hitomado/internal/logging"
)

// logProgress はログインの段階の進み（待受の開始・ブラウザへの受け渡し・ブラウザからの戻り・完了）を Info で記録する。
// 失敗したときにどの段階まで進んだかを、ログから追えるようにする。値（コード・state・nonce・トークン・URL）は載せない。
func (m *Manager) logProgress(stage string) {
	m.opts.Logger.Info("chatgpt: "+stage, "stage", stage)
}

// logFailure はログインの段階 stage での失敗を記録する。画面側が AI_FAILED・INTERNAL などに丸める前の、元のエラーを残す。
// 利用者の操作・想定内の結果（中止・拒否・時間切れ・プランの未許可・別のログインが進行中）は Warn、それ以外は Error。
// OpenAI の応答なら HTTP ステータスと OAuth の error コードを付ける（応答本文・error_description は載せない）。
func (m *Manager) logFailure(stage string, err error, extra ...any) {
	level := slog.LevelError
	switch {
	case errors.Is(err, ErrCancelled), errors.Is(err, ErrDenied), errors.Is(err, ErrTimeout),
		errors.Is(err, ErrPlanNotGranted), errors.Is(err, ErrBusy),
		errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		level = slog.LevelWarn
	}
	attrs := append([]any{"stage", stage, "err", err, "chain", logging.ErrorChain(err)}, extra...)
	var se *serverError
	if errors.As(err, &se) {
		attrs = append(attrs, "status", se.Status)
		if se.Code != "" {
			attrs = append(attrs, "oauth_error", se.Code)
		}
	}
	m.opts.Logger.Log(context.Background(), level, "chatgpt: failed at "+stage, attrs...)
}
