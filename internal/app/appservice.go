package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// AppService はアプリ設定（受信箱フィルタ等）・バージョン・更新確認（バインディング仕様 §7）。
type AppService struct{ d *Deps }

// NewAppService は AppService を作る。
func NewAppService(d *Deps) *AppService { return &AppService{d: d} }

func (s *AppService) GetInboxFilter(ctx context.Context) (dto.InboxFilter, error) {
	return dto.InboxFilter{}, errNotImplemented
}

func (s *AppService) SetInboxFilter(ctx context.Context, filter dto.InboxFilter) error {
	return errNotImplemented
}

func (s *AppService) GetAppInfo(ctx context.Context) (dto.AppInfo, error) {
	return dto.AppInfo{}, errNotImplemented
}

func (s *AppService) SetUpdateCheckEnabled(ctx context.Context, enabled bool) error {
	return errNotImplemented
}

func (s *AppService) CheckForUpdate(ctx context.Context) (dto.UpdateInfo, error) {
	return dto.UpdateInfo{}, errNotImplemented
}
