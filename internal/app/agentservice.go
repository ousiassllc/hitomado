package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/agentcore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// AgentService はエージェントモード（#372）の設定・確認待ちの一覧・質問への答え（バインディング仕様 §9）。
// Wails のバインディング面で、実装は agentcore に委譲する。エージェントは送信しない（送信は ComposeService を人が操作したときだけ）。
type AgentService struct{ core *agentcore.Service }

// NewAgentService は AgentService を作る。
func NewAgentService(d *Deps) *AgentService {
	return &AgentService{core: agentcore.New((*deps.Deps)(d))}
}

// GetAgentSettings はエージェントモード・マスコットの設定と、AI が使える状態かを返す。既定はすべてオフ。
func (s *AgentService) GetAgentSettings(ctx context.Context) (dto.AgentSettings, error) {
	return s.core.GetSettings(ctx)
}

// SetAgentEnabled はエージェントモードのオン・オフ。オンにするには agreed（新着メールの内容を設定済みの AI へ送ることへの同意）が真で、AI が設定済みであること。
func (s *AgentService) SetAgentEnabled(ctx context.Context, enabled, agreed bool) (dto.AgentSettings, error) {
	return s.core.SetEnabled(ctx, enabled, agreed)
}

// SetAgentMascot は画面の端のマスコット（Windows）のオン・オフ。
func (s *AgentService) SetAgentMascot(ctx context.Context, enabled bool) (dto.AgentSettings, error) {
	return s.core.SetMascot(ctx, enabled)
}

// ListAgentItems はエージェントが扱った新着（確認待ち・質問待ち・返信不要・あとで・確認済み・失敗）を新しい順に返す。
func (s *AgentService) ListAgentItems(ctx context.Context) (dto.AgentOverview, error) {
	return s.core.ListItems(ctx)
}

// AnswerAgentQuestions は質問への答えを受けて、下書きを作る（または作り直す）。answers は質問と同じ順・同じ件数。
func (s *AgentService) AnswerAgentQuestions(ctx context.Context, itemID string, answers []string) (dto.AgentItem, error) {
	return s.core.AnswerQuestions(ctx, itemID, answers)
}

// DeferAgentItem は確認待ち・質問待ちを「あとで」にする。
func (s *AgentService) DeferAgentItem(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return s.core.Defer(ctx, itemID)
}

// ConfirmAgentItem は利用者が下書きを開いて確認したことを記録する（確認済みにする）。
func (s *AgentService) ConfirmAgentItem(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return s.core.Confirm(ctx, itemID)
}

// ReconsiderAgentItem は「やっぱり返信する」。返信不要と判断したもの・失敗したものを、返信が要る前提で下書き作り（必要なら質問）からやり直す。
func (s *AgentService) ReconsiderAgentItem(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return s.core.Reconsider(ctx, itemID)
}

// OpenAgentPanel はメインウィンドウを前面に出し、エージェントのパネルを開かせる（マスコットの押下）。
func (s *AgentService) OpenAgentPanel(ctx context.Context) error { return s.core.OpenPanel(ctx) }
