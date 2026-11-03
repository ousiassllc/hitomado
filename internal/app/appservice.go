package app

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/store"
	"github.com/ousiassllc/hitomado/internal/update"
)

// AppService はアプリ設定（受信箱フィルタ等）・バージョン・更新確認（バインディング仕様 §7）。
type AppService struct{ d *Deps }

// NewAppService は AppService を作る。
func NewAppService(d *Deps) *AppService { return &AppService{d: d} }

// GetInboxFilter は保存済みフィルタを返す。存在しないアカウント ID は除く。
func (s *AppService) GetInboxFilter(ctx context.Context) (dto.InboxFilter, error) {
	filter := dto.InboxFilter{AccountIDs: []string{}}
	raw, ok, err := s.d.Store.GetSetting(ctx, store.SettingInboxFilter)
	if err != nil {
		return dto.InboxFilter{}, apperr.FromInfra(err, "")
	}
	if ok {
		if err := json.Unmarshal([]byte(raw), &filter); err != nil {
			// 壊れた保存値は既定に戻す（利用者操作で上書きされる）。
			logger(s.d).Warn("保存済みの受信箱フィルタを読めません。既定を使います", "err", err)
			filter = dto.InboxFilter{}
		}
	}
	_, byID, err := accountsByID(s.d, ctx)
	if err != nil {
		return dto.InboxFilter{}, err
	}
	ids := make([]string, 0, len(filter.AccountIDs))
	for _, id := range filter.AccountIDs {
		if _, ok := byID[id]; ok && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	filter.AccountIDs = ids
	return filter, nil
}

// SetInboxFilter はフィルタを保存する。存在しないアカウント ID は除いて保存する。
func (s *AppService) SetInboxFilter(ctx context.Context, filter dto.InboxFilter) error {
	_, byID, err := accountsByID(s.d, ctx)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(filter.AccountIDs))
	for _, id := range filter.AccountIDs {
		if _, ok := byID[id]; ok && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	b, err := json.Marshal(dto.InboxFilter{AccountIDs: ids, UnreadOnly: filter.UnreadOnly})
	if err != nil {
		return apperr.FromInfra(err, "")
	}
	if err := s.d.Store.SetSetting(ctx, store.SettingInboxFilter, string(b)); err != nil {
		return apperr.FromInfra(err, "")
	}
	return nil
}

// GetAppInfo はバージョン・更新確認の有効／無効・データ保存先を返す。
func (s *AppService) GetAppInfo(ctx context.Context) (dto.AppInfo, error) {
	enabled, err := s.updateCheckEnabled(ctx)
	if err != nil {
		return dto.AppInfo{}, err
	}
	return dto.AppInfo{Version: s.d.Version, UpdateCheckEnabled: enabled, DataDir: s.d.DataDir}, nil
}

// updateCheckEnabled は更新確認の設定（未設定は有効）。
func (s *AppService) updateCheckEnabled(ctx context.Context) (bool, error) {
	raw, ok, err := s.d.Store.GetSetting(ctx, store.SettingUpdateCheck)
	if err != nil {
		return false, apperr.FromInfra(err, "")
	}
	if !ok {
		return true, nil
	}
	enabled, perr := strconv.ParseBool(raw)
	if perr != nil {
		return true, nil // 壊れた値は既定（有効）
	}
	return enabled, nil
}

// SetUpdateCheckEnabled は更新確認のオン／オフを保存する。
func (s *AppService) SetUpdateCheckEnabled(ctx context.Context, enabled bool) error {
	if err := s.d.Store.SetSetting(ctx, store.SettingUpdateCheck, strconv.FormatBool(enabled)); err != nil {
		return apperr.FromInfra(err, "")
	}
	return nil
}

// CheckForUpdate は更新確認を今すぐ行う。無効のとき、および dev・ローカルビルド（版 update.DevVersion）のときは通信せず {available:false}。
func (s *AppService) CheckForUpdate(ctx context.Context) (dto.UpdateInfo, error) {
	if update.IsDev(s.d.Version) {
		return dto.UpdateInfo{}, nil
	}
	enabled, err := s.updateCheckEnabled(ctx)
	if err != nil || !enabled {
		return dto.UpdateInfo{}, err
	}
	rel, err := s.d.Update.Latest(ctx)
	if err != nil {
		return dto.UpdateInfo{}, apperr.FromInfra(err, "")
	}
	if err := s.d.Store.SetSetting(ctx, store.SettingLastChecked, strconv.FormatInt(s.d.Clock.Now().UnixMilli(), 10)); err != nil {
		logger(s.d).Warn("最終確認日時の保存に失敗", "err", err)
	}
	return updateInfo(s.d.Version, rel), nil
}

func updateInfo(current string, rel update.Release) dto.UpdateInfo {
	if !update.IsNewer(current, rel.Version) {
		return dto.UpdateInfo{}
	}
	return dto.UpdateInfo{Available: true, Version: rel.Version, URL: rel.URL}
}

// AckBeforeQuit は終了前の保存が済んだことをトレイの終了処理（platform.Quitter）へ知らせる。
// フロントは events.BeforeQuit を受けて未保存の下書きを保存してから呼ぶ（F-12）。
func (s *AppService) AckBeforeQuit(_ context.Context) error {
	if s.d.QuitAck != nil {
		s.d.QuitAck()
	}
	return nil
}
