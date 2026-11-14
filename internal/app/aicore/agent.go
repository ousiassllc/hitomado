package aicore

import (
	"context"
	"errors"
	"strings"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// エージェントモード（#372）が使う、セッション・リクエスト ID を要さない入口。
// 前提の検査・同意・ChatGPT の 401 更新・タイムアウト・エラー写像は GenerateReplyDraft と同じもの（precheck・generate）を共有する。
// 中止（CancelGenerate）は持たない。ctx の終了で止まる。

// Ready は AI が使える状態か（プロバイダ・モデル・キー／ChatGPT のログイン・現在の送信先への同意）。
// 未設定・未同意は false（エラーにしない）。ストアの読み取りの失敗などは err。通信はしない。
func (s *Service) Ready(ctx context.Context) (bool, error) {
	_, _, err := s.precheckReady(ctx)
	var ae *apperr.Error
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &ae) && (ae.Code == apperr.AINotConfigured || ae.Code == apperr.AIConsentRequired || ae.Code == apperr.Validation):
		// Validation は保存済みの Base URL が使えないとき。設定を直すまで使えない状態として扱う。
		return false, nil
	default:
		return false, err
	}
}

// AgreeCurrent は AI が設定済み（同意は問わない）のとき、現在のプロバイダ・送信先への同意を記録する（RecordAIConsent と同じ保存）。
// 未設定は AI_NOT_CONFIGURED。エージェントモードをオンにするときの同意に使う。
func (s *Service) AgreeCurrent(ctx context.Context) error {
	cfg, _, err := s.precheckConfigured(ctx)
	if err != nil {
		return err
	}
	_, err = s.RecordAIConsent(ctx, cfg.Provider, currentBaseURL(cfg, cfg.Provider))
	return err
}

// JudgeThread は ref のスレッドの最後のメッセージに返信が要るかを AI に判断させる。
func (s *Service) JudgeThread(ctx context.Context, ref dto.ThreadRef) (ai.ReplyJudgment, error) {
	cfg, key, err := s.precheck(ctx)
	if err != nil {
		return ai.ReplyJudgment{}, s.precheckError(ctx, err)
	}
	acct, msgs, err := s.replyContext(ctx, ref)
	if err != nil {
		return ai.ReplyJudgment{}, s.mapError(ctx, err, ref.AccountID)
	}
	in := ai.ReplyJudgeInput{AccountDisplayName: acct.DisplayName, AccountEmail: acct.Email, Effort: cfg.Effort, Messages: msgs}
	var judgment ai.ReplyJudgment
	_, err = s.generate(ctx, cfg, key, in.Effort, func(ctx context.Context, p ai.Provider) (string, error) {
		var err error
		judgment, err = p.JudgeReply(ctx, in)
		return "", err
	})
	return judgment, err
}

// DraftReplyText は ref のスレッドへの返信本文（プレーンテキスト。署名・引用を含まない）を AI で作る。
// instruction は AI への指示（利用者の回答など。呼び出し側が組み立てる。空なら指示なし）。
func (s *Service) DraftReplyText(ctx context.Context, ref dto.ThreadRef, instruction string) (string, error) {
	cfg, key, err := s.precheck(ctx)
	if err != nil {
		return "", s.precheckError(ctx, err)
	}
	in, err := s.buildInput(ctx, ref)
	if err != nil {
		return "", s.mapError(ctx, err, ref.AccountID)
	}
	in.Instruction = strings.TrimSpace(instruction)
	in.Effort = cfg.Effort
	text, err := s.generate(ctx, cfg, key, in.Effort, func(ctx context.Context, p ai.Provider) (string, error) {
		return p.GenerateReply(ctx, in)
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", apperr.Wrap(apperr.AIFailed, "AI から本文を得られませんでした。", ai.ErrFailed)
	}
	return text, nil
}
