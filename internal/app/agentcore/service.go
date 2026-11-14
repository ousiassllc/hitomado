// Package agentcore はエージェントモード（#372）の本体。新着メールの返信要否の判断・返信下書きの作成・利用者への質問・確認待ちの一覧。
// 送信はしない（このパッケージは送信の経路を持たない）。
package agentcore

import (
	"context"
	"errors"

	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// Service はエージェントモードのサービス。
type Service struct {
	d *deps.Deps
}

// New は Service を作る。
func New(d *deps.Deps) *Service { return &Service{d: d} }

var errUnimplemented = errors.New("agentcore: 未実装")

func (s *Service) GetSettings(ctx context.Context) (dto.AgentSettings, error) {
	return dto.AgentSettings{}, errUnimplemented
}

func (s *Service) SetEnabled(ctx context.Context, enabled, agreed bool) (dto.AgentSettings, error) {
	return dto.AgentSettings{}, errUnimplemented
}

func (s *Service) SetMascot(ctx context.Context, enabled bool) (dto.AgentSettings, error) {
	return dto.AgentSettings{}, errUnimplemented
}

func (s *Service) ListItems(ctx context.Context) (dto.AgentOverview, error) {
	return dto.AgentOverview{}, errUnimplemented
}

func (s *Service) AnswerQuestions(ctx context.Context, itemID string, answers []string) (dto.AgentItem, error) {
	return dto.AgentItem{}, errUnimplemented
}

func (s *Service) Defer(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return dto.AgentItem{}, errUnimplemented
}

func (s *Service) Confirm(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return dto.AgentItem{}, errUnimplemented
}

func (s *Service) Reconsider(ctx context.Context, itemID string) (dto.AgentItem, error) {
	return dto.AgentItem{}, errUnimplemented
}

func (s *Service) OpenPanel(ctx context.Context) error { return errUnimplemented }
