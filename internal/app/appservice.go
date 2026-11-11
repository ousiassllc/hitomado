package app

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/app/updatecore"
	"github.com/ousiassllc/hitomado/internal/store"
)

// AppService はアプリ設定（受信箱フィルタ等）・バージョン・更新確認（バインディング仕様 §7）。
type AppService struct {
	d       *Deps
	updates *updatecore.Service
}

// NewAppService は AppService を作る。
func NewAppService(d *Deps) *AppService {
	return &AppService{d: d, updates: updatecore.New((*deps.Deps)(d))}
}

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

// GetAppInfo はバージョン・更新確認と自動アップデートの有効／無効・データ保存先を返す。
func (s *AppService) GetAppInfo(ctx context.Context) (dto.AppInfo, error) {
	check, err := s.updates.CheckEnabled(ctx)
	if err != nil {
		return dto.AppInfo{}, err
	}
	auto, err := s.updates.AutoEnabled(ctx)
	if err != nil {
		return dto.AppInfo{}, err
	}
	confirm, err := composecore.SendConfirmEnabled(ctx, (*deps.Deps)(s.d))
	if err != nil {
		return dto.AppInfo{}, err
	}
	return dto.AppInfo{
		Version: s.d.Version, UpdateCheckEnabled: check,
		AutoUpdateSupported: s.updates.Supported(), AutoUpdateEnabled: auto, DataDir: s.d.DataDir,
		SendConfirmEnabled: confirm,
	}, nil
}

// SetSendConfirmEnabled は送信前に確認画面を出すか（既定オン）を保存する。オフでも、送信元は受信したアカウントに
// 固定され、送信は PrepareSend の確認トークンを使った Send を人がボタンを押したときだけ行う（#313）。
func (s *AppService) SetSendConfirmEnabled(ctx context.Context, enabled bool) error {
	return composecore.SetSendConfirmEnabled(ctx, (*deps.Deps)(s.d), enabled)
}

// SetUpdateCheckEnabled は更新確認のオン／オフを保存する。アプリ内の更新確認を持たない OS（Android）では UNSUPPORTED。
func (s *AppService) SetUpdateCheckEnabled(ctx context.Context, enabled bool) error {
	return s.updates.SetCheckEnabled(ctx, enabled)
}

// SetAutoUpdateEnabled は自動アップデートのオン／オフを保存する。オフにすると、用意済みの更新ファイルを捨て、
// 以後は新しい版のリンクを出すだけになる。入れ替えを行わないビルドでは UNSUPPORTED。
func (s *AppService) SetAutoUpdateEnabled(ctx context.Context, enabled bool) error {
	return s.updates.SetAutoEnabled(ctx, enabled)
}

// GetUpdateStatus は自動アップデートの現在の状態（ダウンロード中・入れ替え待ち・失敗・なし）を返す。
func (s *AppService) GetUpdateStatus(_ context.Context) (dto.UpdateStatus, error) {
	return s.updates.Status(), nil
}

// RestartToUpdate は「再起動して更新」。ダウンロードと照合が済んだ更新があるときに、アプリを終了して入れ替え、
// インストール後に起動し直す。送信中・下書きの保存中は VALIDATION（入れ替えない）。
func (s *AppService) RestartToUpdate(_ context.Context) error { return s.updates.RestartToUpdate() }

// CheckForUpdate は更新確認を今すぐ行う。新しい版があり自動アップデートが有効なら、更新ファイルのダウンロードと照合も始める（応答は待たない）。
// 無効のとき（アプリ内の更新確認を持たない OS を含む）、および dev・ローカルビルド（版 update.DevVersion）のときは通信せず {available:false}。
func (s *AppService) CheckForUpdate(ctx context.Context) (dto.UpdateInfo, error) {
	return s.updates.Check(ctx)
}

// AckBeforeQuit は終了前の保存が済んだことをトレイの終了処理（platform.Quitter）へ知らせる。
// フロントは events.BeforeQuit を受けて未保存の下書きを保存してから呼ぶ（F-12）。
func (s *AppService) AckBeforeQuit(_ context.Context) error {
	if s.d.QuitAck != nil {
		s.d.QuitAck()
	}
	return nil
}
