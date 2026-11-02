package aicore

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/bodyhtml"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

// AI に渡すコンテキストの上限（NF §12）。
const (
	aiContextMessages = 5
	aiContextChars    = 20000
)

// labelDraft は Gmail の下書きメッセージに付くシステムラベル。
const labelDraft = "DRAFT"

// errCancelledByUser は CancelGenerate による中止の原因。
var errCancelledByUser = errors.New("ai: cancelled by user")

// GenerateReplyDraft は reply／reply_all セッションのスレッドから返信本文を生成する（F-08）。
// 署名・引用は含めない。ctx のキャンセルと CancelGenerate は CANCELLED、60 秒超過は AI_FAILED。
func (s *Service) GenerateReplyDraft(ctx context.Context, sessionID, requestID string) (dto.ReplyDraft, error) {
	if requestID == "" {
		return dto.ReplyDraft{}, apperr.New(apperr.Validation, "リクエスト ID が必要です。")
	}
	mode, ref, err := s.sessions.ThreadRef(sessionID)
	if err != nil {
		return dto.ReplyDraft{}, err
	}
	if mode != model.ModeReply && mode != model.ModeReplyAll {
		return dto.ReplyDraft{}, apperr.New(apperr.Validation, "AI 下書きは返信・全員に返信でのみ使えます。")
	}
	cfg, key, err := s.precheck(ctx)
	if err != nil {
		return dto.ReplyDraft{}, err
	}

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if err := s.register(requestID, cancel); err != nil {
		return dto.ReplyDraft{}, err
	}
	defer s.unregister(requestID)

	in, err := s.buildInput(runCtx, ref)
	if err != nil {
		return dto.ReplyDraft{}, s.mapError(runCtx, err, ref.AccountID)
	}
	provider, err := s.d.AI(cfg.Provider, cfg.Model, cfg.BaseURL, key)
	if err != nil {
		return dto.ReplyDraft{}, apperr.Wrap(apperr.AIFailed, "AI プロバイダを初期化できません。設定を確認してください。", err)
	}
	callCtx, stop := context.WithTimeout(runCtx, s.timeout)
	defer stop()
	text, err := provider.GenerateReply(callCtx, in)
	if err != nil {
		return dto.ReplyDraft{}, s.mapError(callCtx, err, "")
	}
	if strings.TrimSpace(text) == "" {
		return dto.ReplyDraft{}, apperr.Wrap(apperr.AIFailed, "AI から本文を得られませんでした。", ai.ErrFailed)
	}
	return dto.ReplyDraft{BodyHTML: bodyhtml.TextToHTML(text)}, nil
}

// precheck は前提条件（キー・モデル設定済み→同意済み）を検査する。キーはここ以外に渡さない。
func (s *Service) precheck(ctx context.Context) (storedAIConfig, string, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return cfg, "", err
	}
	key, ok, err := s.apiKey(cfg.Provider)
	if err != nil {
		return cfg, "", err
	}
	if !ok || strings.TrimSpace(cfg.Model) == "" {
		return cfg, "", apperr.New(apperr.AINotConfigured, "AI の API キーとモデルを設定画面で設定してください。")
	}
	consented, err := s.loadConsent(ctx)
	if err != nil {
		return cfg, "", err
	}
	if !slices.Contains(consented, cfg.Provider) {
		return cfg, "", apperr.New(apperr.AIConsentRequired, "スレッドの内容が AI プロバイダへ送信されることへの同意が必要です。")
	}
	return cfg, key, nil
}

// buildInput はスレッドから AI への入力を作る（DRAFT を除き、テキスト化し、5 件・20,000 文字へ切り詰める）。
func (s *Service) buildInput(ctx context.Context, ref dto.ThreadRef) (ai.ReplyInput, error) {
	acct, err := s.d.Store.GetAccount(ctx, ref.AccountID)
	if err != nil {
		return ai.ReplyInput{}, err
	}
	thread, err := s.d.Threads.Get(ctx, ref.AccountID, ref.ThreadID)
	if err != nil {
		return ai.ReplyInput{}, err
	}
	msgs := make([]ai.ContextMessage, 0, len(thread.Messages))
	for _, m := range thread.Messages {
		if slices.Contains(m.LabelIDs, labelDraft) {
			continue
		}
		msgs = append(msgs, ai.ContextMessage{From: senderLabel(m), Date: m.InternalDate, Text: messageText(m)})
	}
	return ai.ReplyInput{
		AccountDisplayName: acct.DisplayName,
		SignatureText:      strings.TrimSpace(textconv.HTMLToText(acct.SignatureHTML)),
		Messages:           ai.Truncate(msgs, aiContextMessages, aiContextChars),
	}, nil
}

func senderLabel(m gmail.MessageFull) string {
	if m.From.Name != "" {
		return m.From.Name
	}
	return m.From.Email
}

func messageText(m gmail.MessageFull) string {
	if m.BodyHTML != "" {
		return strings.TrimSpace(textconv.HTMLToText(m.BodyHTML))
	}
	return strings.TrimSpace(m.BodyText)
}

// mapError は生成中のエラーを AppError にする。中止は CANCELLED、60 秒超過は AI_FAILED。
func (s *Service) mapError(ctx context.Context, err error, accountID string) error {
	cause := context.Cause(ctx)
	switch {
	case errors.Is(cause, errCancelledByUser), errors.Is(cause, context.Canceled):
		return apperr.Wrap(apperr.Cancelled, "AI 下書きの生成を中止しました。", err)
	case errors.Is(cause, context.DeadlineExceeded) && !errors.Is(err, ai.ErrUnauthorized):
		ae := apperr.Wrap(apperr.AIFailed, "AI の応答が時間内に返りませんでした。", err)
		ae.Retryable = true
		return ae
	}
	return apperr.FromInfra(err, accountID)
}

func (s *Service) register(requestID string, cancel context.CancelCauseFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.inflight[requestID]; dup {
		return apperr.New(apperr.Validation, "同じリクエスト ID の生成が実行中です。")
	}
	s.inflight[requestID] = cancel
	return nil
}

func (s *Service) unregister(requestID string) {
	s.mu.Lock()
	delete(s.inflight, requestID)
	s.mu.Unlock()
}

// CancelGenerate は実行中の生成を中止する（GenerateReplyDraft は CANCELLED で終わる）。未知の ID は何もしない。
func (s *Service) CancelGenerate(requestID string) error {
	s.mu.Lock()
	cancel := s.inflight[requestID]
	s.mu.Unlock()
	if cancel != nil {
		cancel(errCancelledByUser)
	}
	return nil
}
