package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// AIService は AI 設定・同意・下書き生成（バインディング仕様 §6）。
type AIService struct{}

func (s *AIService) GetAIConfig(ctx context.Context) (dto.AIConfig, error) {
	return dto.AIConfig{}, errNotImplemented
}

func (s *AIService) SetAIConfig(ctx context.Context, provider, model, baseURL string) (dto.AIConfig, error) {
	return dto.AIConfig{}, errNotImplemented
}

func (s *AIService) SetAIApiKey(ctx context.Context, provider, apiKey string) (dto.AIConfig, error) {
	return dto.AIConfig{}, errNotImplemented
}

func (s *AIService) ClearAIApiKey(ctx context.Context, provider string) (dto.AIConfig, error) {
	return dto.AIConfig{}, errNotImplemented
}

func (s *AIService) RecordAIConsent(ctx context.Context, provider string) (dto.AIConfig, error) {
	return dto.AIConfig{}, errNotImplemented
}

// GenerateReplyDraft は reply／reply_all セッションのスレッドから下書きを生成する。ctx キャンセルは CANCELLED。
func (s *AIService) GenerateReplyDraft(ctx context.Context, sessionID, requestID string) (dto.ReplyDraft, error) {
	return dto.ReplyDraft{}, errNotImplemented
}

func (s *AIService) CancelGenerate(requestID string) error { return errNotImplemented }
