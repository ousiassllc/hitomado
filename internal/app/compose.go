package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// ComposeService は作成セッション・下書き・送信前確認・送信・宛先候補（バインディング仕様 §5）。Wails のバインディング面で、実装は composecore に委譲する。
type ComposeService struct {
	core                     *composecore.Service
	composecore.DraftListAPI // 下書きの一覧と、一覧からの破棄（#366）
}

// NewComposeService は ComposeService を作る。
func NewComposeService(d *Deps) *ComposeService {
	core := composecore.New((*deps.Deps)(d))
	return &ComposeService{core: core, DraftListAPI: core}
}

func (s *ComposeService) OpenCompose(ctx context.Context, req dto.OpenComposeRequest) (dto.ComposeSession, error) {
	return s.core.OpenCompose(ctx, req)
}

func (s *ComposeService) SetComposeSender(ctx context.Context, sessionID, accountID string) (dto.ComposeSession, error) {
	return s.core.SetComposeSender(ctx, sessionID, accountID)
}

func (s *ComposeService) AddAttachments(ctx context.Context, sessionID string, paths []string) ([]dto.ComposeAttachment, error) {
	return s.core.AddAttachments(ctx, sessionID, paths)
}

func (s *ComposeService) RemoveAttachment(ctx context.Context, sessionID, attachmentID string) ([]dto.ComposeAttachment, error) {
	return s.core.RemoveAttachment(ctx, sessionID, attachmentID)
}

func (s *ComposeService) SaveDraft(ctx context.Context, sessionID string, content dto.ComposeContent) (dto.SaveDraftResult, error) {
	return s.core.SaveDraft(ctx, sessionID, content)
}

func (s *ComposeService) PrepareSend(ctx context.Context, sessionID string, content dto.ComposeContent, options dto.SendOptions) (dto.SendPreview, error) {
	return s.core.PrepareSend(ctx, sessionID, content, options)
}

func (s *ComposeService) Send(ctx context.Context, sessionID, confirmToken string, content dto.ComposeContent) (dto.SendResult, error) {
	return s.core.Send(ctx, sessionID, confirmToken, content)
}

func (s *ComposeService) CloseCompose(ctx context.Context, sessionID string) error {
	return s.core.CloseCompose(ctx, sessionID)
}

func (s *ComposeService) DiscardCompose(ctx context.Context, sessionID string) error {
	return s.core.DiscardCompose(ctx, sessionID)
}

func (s *ComposeService) SuggestContacts(ctx context.Context, accountID, prefix string, limit int) ([]model.Address, error) {
	return s.core.SuggestContacts(ctx, accountID, prefix, limit)
}

// nonNilAddrs は nil を空スライスにする（Wails の TS 生成で null にしないため。thread.go とも共用）。
func nonNilAddrs(a []model.Address) []model.Address {
	if a == nil {
		return []model.Address{}
	}
	return a
}
