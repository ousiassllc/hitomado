package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// ThreadService はスレッド詳細・添付保存・リンクを開く（バインディング仕様 §4）。
type ThreadService struct{}

func (s *ThreadService) GetThread(ctx context.Context, ref dto.ThreadRef) (dto.ThreadDetail, error) {
	return dto.ThreadDetail{}, errNotImplemented
}

func (s *ThreadService) SaveAttachment(ctx context.Context, ref dto.MessageRef, attachmentID, suggestedName string) (dto.SaveAttachmentResult, error) {
	return dto.SaveAttachmentResult{}, errNotImplemented
}

// OpenExternal は http／https／mailto のみ許可する（NF-S07）。
func (s *ThreadService) OpenExternal(ctx context.Context, url string) error {
	return errNotImplemented
}
