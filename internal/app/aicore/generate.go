package aicore

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/bodyhtml"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/siwc"
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
// instruction は利用者が書いた「AI への指示」（#109）。前後の空白を除いて空なら指示なし。
// 2,000 文字（rune 数）を超えると、通信せず VALIDATION。指示は保存せず、ログ・エラー文面にも残さない。
func (s *Service) GenerateReplyDraft(ctx context.Context, sessionID, requestID, instruction string) (dto.ReplyDraft, error) {
	if requestID == "" {
		return dto.ReplyDraft{}, apperr.New(apperr.Validation, "リクエスト ID が必要です。")
	}
	instruction = strings.TrimSpace(instruction)
	if utf8.RuneCountInString(instruction) > ai.MaxInstructionRunes {
		return dto.ReplyDraft{}, apperr.New(apperr.Validation, fmt.Sprintf("AI への指示は %d 文字までです。", ai.MaxInstructionRunes))
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
	in.Instruction = instruction
	in.Effort = cfg.Effort
	text, err := s.generate(runCtx, cfg, key, in.Effort, func(ctx context.Context, p ai.Provider) (string, error) {
		return p.GenerateReply(ctx, in)
	})
	if err != nil {
		return dto.ReplyDraft{}, err
	}
	if strings.TrimSpace(text) == "" {
		return dto.ReplyDraft{}, apperr.Wrap(apperr.AIFailed, "AI から本文を得られませんでした。", ai.ErrFailed)
	}
	return dto.ReplyDraft{BodyHTML: bodyhtml.TextToHTML(text)}, nil
}

// providerCall はプロバイダの 1 回の呼び出し（GenerateReply／SummarizeThread／AskAboutThread のどれか）。
type providerCall func(ctx context.Context, p ai.Provider) (string, error)

// generate はプロバイダを作って call を 1 回呼ぶ。ChatGPT で 401 を受けたときだけ、トークンを更新して 1 度だけやり直す
// （更新できなければ再ログインを案内する）。effort は要求に載せたエフォート（拒否されたときの案内用）。返すエラーは AppError。
func (s *Service) generate(ctx context.Context, cfg storedAIConfig, key, effort string, call providerCall) (string, error) {
	text, err := s.callProvider(ctx, cfg, key, call)
	if err == nil || cfg.Provider != providerChatGPT || !errors.Is(err, ai.ErrUnauthorized) || ctx.Err() != nil {
		return text, s.mapProviderError(cfg.Provider, effort, err)
	}
	key, rerr := s.d.ChatGPT.ForceRefresh(ctx)
	if rerr != nil {
		if context.Cause(ctx) != nil {
			return "", s.mapError(ctx, rerr, "") // 中止は CANCELLED、期限切れは AI_FAILED
		}
		return "", mapChatGPTError(rerr)
	}
	text, err = s.callProvider(ctx, cfg, key, call)
	return text, s.mapProviderError(cfg.Provider, effort, err)
}

// callProvider はプロバイダを作って call を呼ぶ。プロバイダのエラーは生のまま返す（呼び出し側が分類する）。
// 中止・期限切れ（ctx の終了）と、プロバイダを作れないエラーだけは、ここで AppError にする。
func (s *Service) callProvider(ctx context.Context, cfg storedAIConfig, key string, call providerCall) (string, error) {
	provider, err := s.d.AI(cfg.Provider, cfg.Model, cfg.BaseURL, key)
	if err != nil {
		if errors.Is(err, ai.ErrInvalidBaseURL) {
			return "", apperr.Wrap(apperr.Validation, "Base URL が使えません。設定を確認してください。", err)
		}
		return "", apperr.Wrap(apperr.AIFailed, "AI プロバイダを初期化できません。設定を確認してください。", err)
	}
	callCtx, stop := context.WithTimeout(ctx, s.timeout)
	defer stop()
	text, err := call(callCtx, provider)
	if err != nil {
		if context.Cause(callCtx) != nil {
			return "", s.mapError(callCtx, err, "")
		}
		return "", err
	}
	return text, nil
}

// mapProviderError はプロバイダのエラーを AppError にする。ChatGPT の 401 は API キーではなくログインの問題として伝える。
// effort を載せた要求をプロバイダが拒否したとき（400／422）は、エフォートが原因のことがあるので、設定で「既定」に戻す案内を付ける（#111）。
func (s *Service) mapProviderError(provider, effort string, err error) error {
	if err == nil {
		return nil
	}
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return err
	}
	if effort != "" && errors.Is(err, ai.ErrRejected) {
		return apperr.Wrap(apperr.AIFailed, "このモデルは選んだエフォートに対応していません。設定でエフォートを「既定」に戻してください。", err).
			WithDetail(apperr.DetailReason, apperr.ReasonEffortRejected)
	}
	if provider == providerChatGPT && errors.Is(err, ai.ErrUnauthorized) {
		return apperr.Wrap(apperr.AIFailed, "ChatGPT のログインを確認できませんでした。設定画面でログインし直してください。", err)
	}
	return apperr.FromInfra(err, "")
}

// precheck は前提条件（キー・モデル設定済み→同意済み）を検査する。キーはここ以外に渡さない。
func (s *Service) precheck(ctx context.Context) (storedAIConfig, string, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return cfg, "", err
	}
	// ChatGPT のトークンは、同意の確認が済むまで取得（更新）しない。まずはログイン済みかだけを見る。
	var (
		key string
		ok  bool
	)
	if cfg.Provider == providerChatGPT {
		var gpt siwc.Status
		gpt, err = s.chatGPTStatus(ctx)
		ok = gpt.SignedIn
	} else {
		key, ok, err = s.apiKey(cfg.Provider)
	}
	if err != nil {
		return cfg, "", err
	}
	if !ok || strings.TrimSpace(cfg.Model) == "" {
		if cfg.Provider == providerChatGPT {
			return cfg, "", apperr.New(apperr.AINotConfigured, "ChatGPT にログインし、モデルを設定画面で設定してください。")
		}
		return cfg, "", apperr.New(apperr.AINotConfigured, "AI の API キーとモデルを設定画面で設定してください。")
	}
	if cfg.BaseURL != "" && cfg.Provider == providerOpenAI {
		// 保存済みの旧設定（非 loopback の http:// など）は、接続前に分かりやすく拒否する。
		if err := validateBaseURL(cfg.BaseURL); err != nil {
			return cfg, "", apperr.Wrap(apperr.Validation, "保存済みの Base URL は使えません。設定画面で https:// の URL（または localhost の http://）に直してください。", err)
		}
	}
	consented, err := s.loadConsent(ctx, cfg)
	if err != nil {
		return cfg, "", err
	}
	if !slices.Contains(consented, cfg.Provider) {
		return cfg, "", apperr.New(apperr.AIConsentRequired, "スレッドの内容が AI プロバイダへ送信されることへの同意が必要です。")
	}
	if cfg.Provider == providerChatGPT {
		key, ok, err = s.credential(ctx, cfg.Provider)
		if err != nil {
			return cfg, "", err
		}
		if !ok {
			return cfg, "", apperr.New(apperr.AINotConfigured, "ChatGPT にログインし、モデルを設定画面で設定してください。")
		}
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
	msgs := contextMessages(thread)
	return ai.ReplyInput{
		AccountDisplayName: acct.DisplayName,
		SignatureText:      strings.TrimSpace(textconv.HTMLToText(acct.SignatureHTML)),
		Messages:           ai.Truncate(msgs, aiContextMessages, aiContextChars),
	}, nil
}

// contextMessages はスレッドの DRAFT でないメッセージを、古い順のままプレーンテキスト化して返す（添付はファイル名とサイズだけ）。
func contextMessages(thread gmail.ThreadFull) []ai.ContextMessage {
	msgs := make([]ai.ContextMessage, 0, len(thread.Messages))
	for _, m := range thread.Messages {
		if slices.Contains(m.LabelIDs, labelDraft) {
			continue
		}
		msgs = append(msgs, ai.ContextMessage{From: senderLabel(m), Date: m.InternalDate, Text: messageText(m), Attachments: attachmentRefs(m.Attachments)})
	}
	return msgs
}

func attachmentRefs(atts []model.AttachmentInfo) []ai.AttachmentRef {
	refs := make([]ai.AttachmentRef, len(atts))
	for i, a := range atts {
		refs[i] = ai.AttachmentRef{Filename: a.Filename, Size: a.Size}
	}
	return refs
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
		return apperr.Wrap(apperr.Cancelled, "AI の生成を中止しました。", err)
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

// CancelGenerate は実行中の生成を中止する（GenerateReplyDraft・SummarizeThread・AskAboutThread・FindInboxZeroCandidates は CANCELLED で終わる）。未知の ID は何もしない。
func (s *Service) CancelGenerate(requestID string) error {
	s.mu.Lock()
	cancel := s.inflight[requestID]
	s.mu.Unlock()
	if cancel != nil {
		cancel(errCancelledByUser)
	}
	return nil
}
