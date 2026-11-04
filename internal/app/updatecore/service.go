// Package updatecore は更新確認（NF-D03）と、Windows 版の自動アップデート（新しい版のダウンロード・照合・入れ替え）。
// internal/app のファイル数・行数の上限のためサブパッケージにしている（AIService の aicore と同じ）。
// AppService（バインディング）と main の終了処理・定期確認ループが使う。
package updatecore

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/store"
	"github.com/ousiassllc/hitomado/internal/update"
)

// DirName はデータフォルダの下の、更新ファイル（ダウンロードしたインストーラー）の保存先フォルダ名。
const DirName = "updates"

// Service は更新確認・自動アップデートの設定と操作。
type Service struct{ d *deps.Deps }

// New は Service を作る。
func New(d *deps.Deps) *Service { return &Service{d: d} }

func (s *Service) log() *slog.Logger {
	if s.d.Logger != nil {
		return s.d.Logger
	}
	return slog.Default()
}

// readBool は真偽の設定を読む。未設定・壊れた値は def。
func (s *Service) readBool(ctx context.Context, key string, def bool) (bool, error) {
	raw, ok, err := s.d.Store.GetSetting(ctx, key)
	if err != nil {
		return false, apperr.FromInfra(err, "")
	}
	if !ok {
		return def, nil
	}
	v, perr := strconv.ParseBool(raw)
	if perr != nil {
		return def, nil
	}
	return v, nil
}

func (s *Service) writeBool(ctx context.Context, key string, v bool) error {
	if err := s.d.Store.SetSetting(ctx, key, strconv.FormatBool(v)); err != nil {
		return apperr.FromInfra(err, "")
	}
	return nil
}

// CheckEnabled は更新確認の設定（未設定は有効）。アプリ内の更新確認を持たない OS（Android）では、保存値にかかわらず常に無効。
func (s *Service) CheckEnabled(ctx context.Context) (bool, error) {
	if !s.d.Capabilities.SupportsAutoUpdate {
		return false, nil
	}
	return s.readBool(ctx, store.SettingUpdateCheck, true)
}

// SetCheckEnabled は更新確認のオン／オフを保存する。オフなら用意済みの更新も入れ替えない。
func (s *Service) SetCheckEnabled(ctx context.Context, enabled bool) error {
	if !s.d.Capabilities.SupportsAutoUpdate {
		return apperr.New(apperr.Unsupported, "この端末ではアプリ内の更新確認を使えません。")
	}
	if err := s.writeBool(ctx, store.SettingUpdateCheck, enabled); err != nil {
		return err
	}
	if !enabled && s.d.Updates != nil {
		s.d.Updates.Discard()
	}
	return nil
}

// Supported はこのビルドが新しい版の自動ダウンロードと入れ替えを行えるか（Windows のリリース版）。
func (s *Service) Supported() bool { return s.d.Updates != nil }

// AutoEnabled は自動アップデートの設定（未設定は有効）。入れ替えを行わないビルドでは、保存値にかかわらず常に無効。
func (s *Service) AutoEnabled(ctx context.Context) (bool, error) {
	if s.d.Updates == nil {
		return false, nil
	}
	return s.readBool(ctx, store.SettingAutoUpdate, true)
}

// SetAutoEnabled は自動アップデートのオン／オフを保存する。オフにすると、用意済みの更新ファイルを捨て、
// 以後は新しい版のリンクを出すだけになる。入れ替えを行わないビルドでは UNSUPPORTED。
func (s *Service) SetAutoEnabled(ctx context.Context, enabled bool) error {
	if s.d.Updates == nil {
		return apperr.New(apperr.Unsupported, "この端末では自動アップデートを使えません。")
	}
	if err := s.writeBool(ctx, store.SettingAutoUpdate, enabled); err != nil {
		return err
	}
	if !enabled {
		s.d.Updates.Discard()
	}
	return nil
}

// Check は更新確認を今すぐ行う。無効のとき（アプリ内の更新確認を持たない OS を含む）、および dev・ローカルビルド
// （版 update.DevVersion）のときは通信せず {available:false}。新しい版があり自動アップデートが有効なら、
// 更新ファイルのダウンロードと照合も始める（応答は待たない。進みは update:status で伝える）。
func (s *Service) Check(ctx context.Context) (dto.UpdateInfo, error) {
	if update.IsDev(s.d.Version) {
		return dto.UpdateInfo{}, nil
	}
	enabled, err := s.CheckEnabled(ctx)
	if err != nil || !enabled {
		return dto.UpdateInfo{}, err
	}
	rel, err := s.d.Update.Latest(ctx)
	if err != nil {
		return dto.UpdateInfo{}, apperr.FromInfra(err, "")
	}
	if err := s.d.Store.SetSetting(ctx, store.SettingLastChecked, strconv.FormatInt(s.d.Clock.Now().UnixMilli(), 10)); err != nil {
		s.log().Warn("最終確認日時の保存に失敗", "err", err)
	}
	if !update.IsNewer(s.d.Version, rel.Version) {
		return dto.UpdateInfo{}, nil
	}
	if s.d.Updates != nil {
		if auto, err := s.AutoEnabled(ctx); err != nil {
			s.log().Warn("自動アップデートの設定を読めません。リンクを出すだけにします", "err", err)
		} else if auto {
			s.d.Updates.PrepareAsync(rel)
		}
	}
	return dto.UpdateInfo{Available: true, Version: rel.Version, URL: rel.URL}, nil
}

// StatusPayload は Manager の状態を画面へ伝える形にする。入れ替え待ち > ダウンロード中 > 失敗 > なし の順で段階を決める。
func StatusPayload(st update.State) events.UpdateStatusPayload {
	switch {
	case st.Staged != nil:
		p := events.UpdateStatusPayload{Stage: events.UpdateStageReady, Version: st.Staged.Version}
		if st.Failure != nil && st.Failure.Version == st.Staged.Version {
			p.Message = st.Failure.Message // 前回の入れ替えが完了しなかった、など
		}
		return p
	case st.Downloading != "":
		return events.UpdateStatusPayload{Stage: events.UpdateStageDownloading, Version: st.Downloading}
	case st.Failure != nil:
		return events.UpdateStatusPayload{Stage: events.UpdateStageFailed, Version: st.Failure.Version, Message: st.Failure.Message}
	}
	return events.UpdateStatusPayload{Stage: events.UpdateStageNone}
}

// Status は自動アップデートの現在の状態（ダウンロード中・入れ替え待ち・失敗・なし）を返す。
func (s *Service) Status() dto.UpdateStatus {
	if s.d.Updates == nil {
		return dto.UpdateStatus{Stage: events.UpdateStageNone}
	}
	return dto.UpdateStatus(StatusPayload(s.d.Updates.State()))
}

// RestartToUpdate は「再起動して更新」。ダウンロードと照合が済んだ更新があるときに、アプリを終了して入れ替え、
// インストール後に起動し直す。送信中・下書きの保存中は VALIDATION（入れ替えない）。
// 終了は未保存の下書きの保存を待ってから行う（トレイの「終了」と同じ）。
func (s *Service) RestartToUpdate() error {
	if s.d.Updates == nil {
		return apperr.New(apperr.Unsupported, "この端末では自動アップデートを使えません。")
	}
	if err := s.d.Updates.RequestRestart(); err != nil {
		switch {
		case errors.Is(err, update.ErrBusy):
			return apperr.Wrap(apperr.Validation, "送信中または下書きの保存中のため、いまは更新できません。終わってからもう一度お試しください。", err)
		case errors.Is(err, update.ErrNoUpdate):
			return apperr.Wrap(apperr.Validation, "更新の準備ができていません。", err)
		}
		return apperr.FromInfra(err, "")
	}
	if s.d.RequestQuit != nil {
		s.d.RequestQuit()
	}
	return nil
}

// ApplyOnExit はアプリの終了時（main の終了処理。バインディングには出さない）に呼ぶ。入れ替え待ちの更新があり、
// 利用者が「再起動して更新」を選んでいるか自動アップデートが有効なら、いま安全（送信中・下書きの保存中でない）なときに
// インストーラーを起動する。起動したら true。失敗・安全でないときは記録するだけで、今の版のまま終了する。
func (s *Service) ApplyOnExit(ctx context.Context) bool {
	if s.d.Updates == nil {
		return false
	}
	auto, err := s.AutoEnabled(ctx)
	if err != nil {
		s.log().Warn("自動アップデートの設定を読めません。入れ替えません", "err", err)
		return false
	}
	started, err := s.d.Updates.ApplyOnExit(auto)
	switch {
	case errors.Is(err, update.ErrBusy):
		s.log().Info("送信または下書きの保存の途中のため、更新の入れ替えを次の終了に回します")
	case err != nil:
		s.log().Error("更新の入れ替えを始められませんでした", "err", err)
	case started:
		s.log().Info("更新のインストーラーを起動しました")
	}
	return started
}
