package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// ComposeService は作成セッション・下書き・送信前確認・送信・宛先候補（バインディング仕様 §5）。
type ComposeService struct{}

func (s *ComposeService) OpenCompose(ctx context.Context, req dto.OpenComposeRequest) (dto.ComposeSession, error) {
	return dto.ComposeSession{}, errNotImplemented
}

func (s *ComposeService) SetComposeSender(ctx context.Context, sessionID, accountID string) (dto.ComposeSession, error) {
	return dto.ComposeSession{}, errNotImplemented
}

func (s *ComposeService) AddAttachments(ctx context.Context, sessionID string, paths []string) ([]dto.ComposeAttachment, error) {
	return nil, errNotImplemented
}

func (s *ComposeService) RemoveAttachment(ctx context.Context, sessionID, attachmentID string) ([]dto.ComposeAttachment, error) {
	return nil, errNotImplemented
}

func (s *ComposeService) SaveDraft(ctx context.Context, sessionID string, content dto.ComposeContent) (dto.SaveDraftResult, error) {
	return dto.SaveDraftResult{}, errNotImplemented
}

func (s *ComposeService) PrepareSend(ctx context.Context, sessionID string, content dto.ComposeContent, options dto.SendOptions) (dto.SendPreview, error) {
	return dto.SendPreview{}, errNotImplemented
}

func (s *ComposeService) Send(ctx context.Context, sessionID, confirmToken string, content dto.ComposeContent) (dto.SendResult, error) {
	return dto.SendResult{}, errNotImplemented
}

func (s *ComposeService) CloseCompose(ctx context.Context, sessionID string) error {
	return errNotImplemented
}

func (s *ComposeService) DiscardCompose(ctx context.Context, sessionID string) error {
	return errNotImplemented
}

func (s *ComposeService) SuggestContacts(ctx context.Context, accountID, prefix string, limit int) ([]model.Address, error) {
	return nil, errNotImplemented
}
