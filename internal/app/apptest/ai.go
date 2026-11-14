package apptest

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// AIConfigurer は AI のつなぎ先と機能ごとに使う AI を設定できるもの（aicore.Service・app.AIService）。
type AIConfigurer interface {
	SetAIBaseURL(ctx context.Context, baseURL string) (dto.AIConfig, error)
	SetAIFeature(ctx context.Context, feature, provider, model, effort string) (dto.AIConfig, error)
}

// SetDraftAI は「AI で下書き」に provider・model・effort を選ぶ。provider が openai のときは、OpenAI 互換の Base URL も baseURL（空は既定）にする。
// 旧 SetAIConfig（既定のプロバイダを 1 回で保存）と同じ形で、テストの前提を作るのに使う。
func SetDraftAI(svc AIConfigurer, ctx context.Context, provider, model, baseURL, effort string) (dto.AIConfig, error) {
	if provider == "openai" {
		if _, err := svc.SetAIBaseURL(ctx, baseURL); err != nil {
			return dto.AIConfig{}, err
		}
	}
	return svc.SetAIFeature(ctx, dto.AIFeatureDraft, provider, model, effort)
}
