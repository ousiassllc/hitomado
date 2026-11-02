package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// InboxService は統合受信箱・検索・未読件数・スレッド操作・ラベル（バインディング仕様 §3）。
type InboxService struct{ d *Deps }

// NewInboxService は InboxService を作る。
func NewInboxService(d *Deps) *InboxService { return &InboxService{d: d} }

func (s *InboxService) ListInbox(ctx context.Context, q dto.InboxQuery) (dto.ThreadPage, error) {
	return dto.ThreadPage{}, errNotImplemented
}

func (s *InboxService) Refresh(ctx context.Context, accountIDs []string) (dto.RefreshResponse, error) {
	return dto.RefreshResponse{}, errNotImplemented
}

func (s *InboxService) GetUnreadCounts(ctx context.Context) (map[string]int, error) {
	return nil, errNotImplemented
}

func (s *InboxService) Search(ctx context.Context, q dto.SearchQuery) (dto.ThreadPage, error) {
	return dto.ThreadPage{}, errNotImplemented
}

func (s *InboxService) MarkRead(ctx context.Context, refs []dto.ThreadRef, read bool) ([]dto.ThreadSummary, error) {
	return nil, errNotImplemented
}

func (s *InboxService) Archive(ctx context.Context, refs []dto.ThreadRef) ([]dto.ThreadSummary, error) {
	return nil, errNotImplemented
}

func (s *InboxService) Trash(ctx context.Context, refs []dto.ThreadRef) ([]dto.ThreadSummary, error) {
	return nil, errNotImplemented
}

// ListLabels はユーザー作成ラベル【Could】。
func (s *InboxService) ListLabels(ctx context.Context, accountID string) ([]dto.Label, error) {
	return nil, errNotImplemented
}

// ModifyLabels はユーザー作成ラベルの付与・除去【Could】。システムラベルは VALIDATION。
func (s *InboxService) ModifyLabels(ctx context.Context, ref dto.ThreadRef, addLabelIDs, removeLabelIDs []string) (dto.ThreadSummary, error) {
	return dto.ThreadSummary{}, errNotImplemented
}
