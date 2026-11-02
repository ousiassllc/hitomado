package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// AIService は AI 設定・同意・下書き生成（バインディング仕様 §6）。
// Wails のバインディング面で、実装は aicore に委譲する。
type AIService struct{ core *aicore.Service }

// NewAIService は AIService を作る。返信系セッションの参照に compose を使う。
func NewAIService(d *Deps, compose *ComposeService) *AIService {
	return &AIService{core: aicore.New((*deps.Deps)(d), compose.core)}
}

func (s *AIService) GetAIConfig(ctx context.Context) (dto.AIConfig, error) {
	return s.core.GetAIConfig(ctx)
}

func (s *AIService) SetAIConfig(ctx context.Context, provider, model, baseURL string) (dto.AIConfig, error) {
	return s.core.SetAIConfig(ctx, provider, model, baseURL)
}

func (s *AIService) SetAIApiKey(ctx context.Context, provider, apiKey string) (dto.AIConfig, error) {
	return s.core.SetAIApiKey(ctx, provider, apiKey)
}

func (s *AIService) ClearAIApiKey(ctx context.Context, provider string) (dto.AIConfig, error) {
	return s.core.ClearAIApiKey(ctx, provider)
}

func (s *AIService) RecordAIConsent(ctx context.Context, provider string) (dto.AIConfig, error) {
	return s.core.RecordAIConsent(ctx, provider)
}

// GenerateReplyDraft は reply／reply_all セッションのスレッドから下書きを生成する。ctx キャンセルは CANCELLED。
func (s *AIService) GenerateReplyDraft(ctx context.Context, sessionID, requestID string) (dto.ReplyDraft, error) {
	return s.core.GenerateReplyDraft(ctx, sessionID, requestID)
}

// CancelGenerate は実行中の生成を中止する（GenerateReplyDraft は CANCELLED で終わる）。
func (s *AIService) CancelGenerate(requestID string) error { return s.core.CancelGenerate(requestID) }
