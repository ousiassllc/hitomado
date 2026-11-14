package aicore

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// スレッドの AI 要約・質問に渡すスレッドの内容の上限（NF §12。F-17・F-18）。AI 下書き（aiContextMessages・aiContextChars）とは別の枠。
const threadAIChars = 40000

// SummarizeThread は ref のスレッドの全メッセージ（下書きを除く。合計 40,000 文字を目安）を要約する（F-17）。
// 前提・中止・タイムアウトは GenerateReplyDraft と同じ。要約は保存せず、ログ・エラー文面にも残さない。
func (s *Service) SummarizeThread(ctx context.Context, ref dto.ThreadRef, requestID string) (dto.ThreadAIResult, error) {
	if err := validateRequestID(requestID); err != nil {
		return dto.ThreadAIResult{}, err
	}
	return s.runThreadAI(ctx, ref, requestID, func(in ai.ThreadInput) providerCall {
		return func(ctx context.Context, p ai.Provider) (string, error) { return p.SummarizeThread(ctx, in) }
	})
}

// AskAboutThread は ref のスレッドの内容と history（そのスレッドでそれまでに交わした質問と回答）を文脈に、question へ答える（F-18）。
// Go は会話を持たず、question・history の検証（空・2,000 文字超・会話の形・20 件／20,000 文字超）は通信前に行って VALIDATION にする
// （古い往復を外すのはフロントの責務で、Go は黙って切り詰めない）。質問・会話は保存せず、ログ・エラー文面にも残さない。
func (s *Service) AskAboutThread(ctx context.Context, ref dto.ThreadRef, requestID, question string, history []dto.ThreadAITurn) (dto.ThreadAIResult, error) {
	if err := validateRequestID(requestID); err != nil {
		return dto.ThreadAIResult{}, err
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return dto.ThreadAIResult{}, apperr.New(apperr.Validation, "質問を入力してください。")
	}
	if utf8.RuneCountInString(question) > ai.MaxQuestionRunes {
		return dto.ThreadAIResult{}, apperr.New(apperr.Validation, fmt.Sprintf("質問は %d 文字までです。", ai.MaxQuestionRunes))
	}
	turns, err := validateHistory(history)
	if err != nil {
		return dto.ThreadAIResult{}, err
	}
	return s.runThreadAI(ctx, ref, requestID, func(in ai.ThreadInput) providerCall {
		q := ai.ThreadQuestionInput{ThreadInput: in, History: turns, Question: question}
		return func(ctx context.Context, p ai.Provider) (string, error) { return p.AskAboutThread(ctx, q) }
	})
}

func validateRequestID(requestID string) error {
	if requestID == "" {
		return apperr.New(apperr.Validation, "リクエスト ID が必要です。")
	}
	return nil
}

// validateHistory は会話の形と長さを検証して ai.ThreadTurn にする。nil・空は空の会話。
// 形: user から始まり assistant で終わる交互の並びで、text は空でない。長さ: 20 件（10 往復）以内・text の合計 20,000 文字以内。
func validateHistory(history []dto.ThreadAITurn) ([]ai.ThreadTurn, error) {
	bad := func(msg string) error { return apperr.New(apperr.Validation, msg) }
	if len(history) > ai.MaxHistoryTurns {
		return nil, bad("会話が長すぎます（最新の 10 往復までです）。")
	}
	if len(history)%2 != 0 {
		return nil, bad("会話の形が正しくありません（質問と回答が対になっている必要があります）。")
	}
	turns := make([]ai.ThreadTurn, len(history))
	total := 0
	for i, t := range history {
		want := ai.RoleUser
		if i%2 == 1 {
			want = ai.RoleAssistant
		}
		if t.Role != want {
			return nil, bad("会話の形が正しくありません（質問と回答が交互である必要があります）。")
		}
		if strings.TrimSpace(t.Text) == "" {
			return nil, bad("会話の形が正しくありません（空の発言があります）。")
		}
		total += utf8.RuneCountInString(t.Text)
		turns[i] = ai.ThreadTurn{Role: t.Role, Text: t.Text}
	}
	if total > ai.MaxHistoryRunes {
		return nil, bad("会話が長すぎます（合計 20,000 文字までです）。")
	}
	return turns, nil
}

// runThreadAI は要約・質問に共通の流れ（前提の確認→スレッドの取得→プロバイダの呼び出し）を行う。
// call は組み立てた入力から 1 回のプロバイダ呼び出しを作る。結果が空なら AI_FAILED。
func (s *Service) runThreadAI(ctx context.Context, ref dto.ThreadRef, requestID string, call func(ai.ThreadInput) providerCall) (dto.ThreadAIResult, error) {
	runCtx, finish, err := s.begin(ctx, requestID)
	if err != nil {
		return dto.ThreadAIResult{}, err
	}
	defer finish()
	cfg, key, err := s.precheck(runCtx, dto.AIFeatureDraft)
	if err != nil {
		return dto.ThreadAIResult{}, s.precheckError(runCtx, err)
	}

	in, total, err := s.buildThreadInput(runCtx, ref)
	if err != nil {
		return dto.ThreadAIResult{}, s.mapError(runCtx, err, ref.AccountID)
	}
	in.Effort = cfg.Effort
	text, err := s.generate(runCtx, cfg, key, in.Effort, call(in))
	if err != nil {
		return dto.ThreadAIResult{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return dto.ThreadAIResult{}, apperr.Wrap(apperr.AIFailed, "AI から結果を得られませんでした。", ai.ErrFailed)
	}
	return dto.ThreadAIResult{Text: text, IncludedCount: len(in.Messages), TotalCount: total}, nil
}

// buildThreadInput はスレッドの AI 要約・質問の入力を作る（DRAFT を除き、全メッセージをテキスト化して合計 40,000 文字へ切り詰める）。
// 戻り値の total は切り詰める前（下書きを除く）のメッセージ数。
func (s *Service) buildThreadInput(ctx context.Context, ref dto.ThreadRef) (ai.ThreadInput, int, error) {
	acct, err := s.d.Store.GetAccount(ctx, ref.AccountID)
	if err != nil {
		return ai.ThreadInput{}, 0, err
	}
	thread, err := s.d.Threads.Get(ctx, ref.AccountID, ref.ThreadID)
	if err != nil {
		return ai.ThreadInput{}, 0, err
	}
	msgs := contextMessages(thread)
	return ai.ThreadInput{
		AccountDisplayName: acct.DisplayName,
		AccountEmail:       acct.Email,
		Messages:           ai.Truncate(msgs, len(msgs), threadAIChars),
	}, len(msgs), nil
}
