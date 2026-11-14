package agentcore

import (
	"context"
	"encoding/json"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/store"
)

// storedSettings は app_settings の agent.settings。未保存は全部オフ。
type storedSettings struct {
	Enabled       bool  `json:"enabled"`
	MascotEnabled bool  `json:"mascotEnabled"`
	AgreedAt      int64 `json:"agreedAt,omitempty"` // オンにしたときの同意の時刻（Unix ミリ秒）
}

func (s *Service) loadSettings(ctx context.Context) (storedSettings, error) {
	v, ok, err := s.d.Store.GetSetting(ctx, store.SettingAgent)
	if err != nil {
		return storedSettings{}, apperr.FromInfra(err, "")
	}
	var cfg storedSettings
	if !ok {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(v), &cfg); err != nil {
		return storedSettings{}, apperr.Wrap(apperr.Internal, "エージェントの設定を読み込めません。", err)
	}
	return cfg, nil
}

func (s *Service) saveSettings(ctx context.Context, cfg storedSettings) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return apperr.Wrap(apperr.Internal, "エージェントの設定を保存できません。", err)
	}
	if err := s.d.Store.SetSetting(ctx, store.SettingAgent, string(b)); err != nil {
		return apperr.FromInfra(err, "")
	}
	return nil
}

// applyMascot はマスコットの表示を設定に合わせる（エージェントとマスコットが両方オンのときだけ出す）。
func (s *Service) applyMascot(cfg storedSettings) {
	if s.d.Mascot != nil {
		s.d.Mascot.SetVisible(cfg.Enabled && cfg.MascotEnabled)
	}
}

// settingsDTO は設定に AI の状態を添えて返す形にする。
func (s *Service) settingsDTO(ctx context.Context, cfg storedSettings) (dto.AgentSettings, error) {
	ready, err := s.d.AI.Ready(ctx)
	if err != nil {
		return dto.AgentSettings{}, err
	}
	return dto.AgentSettings{Enabled: cfg.Enabled, MascotEnabled: cfg.MascotEnabled, AIReady: ready}, nil
}

// GetSettings はエージェントモード・マスコットの設定と、AI が使える状態かを返す。
func (s *Service) GetSettings(ctx context.Context) (dto.AgentSettings, error) {
	cfg, err := s.loadSettings(ctx)
	if err != nil {
		return dto.AgentSettings{}, err
	}
	return s.settingsDTO(ctx, cfg)
}

// SetEnabled はエージェントモードのオン・オフ。オンにするには agreed が真で、AI が設定済みであること
// （通れば現在のプロバイダ・送信先への同意を記録する）。オフにするときは agreed を見ない。
func (s *Service) SetEnabled(ctx context.Context, enabled, agreed bool) (dto.AgentSettings, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	cfg, err := s.loadSettings(ctx)
	if err != nil {
		return dto.AgentSettings{}, err
	}
	if enabled {
		if !agreed {
			return dto.AgentSettings{}, apperr.New(apperr.Validation, "エージェントモードをオンにするには、新着メールの内容を設定済みの AI へ送ることへの同意が必要です。")
		}
		if err := s.d.AI.AgreeCurrent(ctx); err != nil {
			return dto.AgentSettings{}, err
		}
		if !cfg.Enabled {
			cfg.AgreedAt = s.d.Clock.Now().UnixMilli()
		}
	}
	cfg.Enabled = enabled
	if err := s.saveSettings(ctx, cfg); err != nil {
		return dto.AgentSettings{}, err
	}
	s.applyMascot(cfg)
	s.emitUpdated(ctx, cfg)
	return s.settingsDTO(ctx, cfg)
}

// SetMascot はマスコットのオン・オフ（エージェントがオンのときだけ表示される）。
func (s *Service) SetMascot(ctx context.Context, enabled bool) (dto.AgentSettings, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	cfg, err := s.loadSettings(ctx)
	if err != nil {
		return dto.AgentSettings{}, err
	}
	cfg.MascotEnabled = enabled
	if err := s.saveSettings(ctx, cfg); err != nil {
		return dto.AgentSettings{}, err
	}
	s.applyMascot(cfg)
	s.emitUpdated(ctx, cfg)
	return s.settingsDTO(ctx, cfg)
}

// emitUpdated は agent:updated を発行する（確認待ち・質問待ちの件数と設定。件名・本文は載せない）。
// 件数を数えられなければ 0 で発行せず、ログだけ残す。
func (s *Service) emitUpdated(ctx context.Context, cfg storedSettings) {
	n, err := s.d.Store.CountPendingAgentItems(ctx)
	if err != nil {
		s.log.Warn("agent: count pending failed", "kind", errKind(err))
		return
	}
	s.d.Events.Emit(events.AgentUpdated, events.AgentUpdatedPayload{PendingCount: n, Enabled: cfg.Enabled, MascotEnabled: cfg.MascotEnabled})
}

// emitUpdatedNow は設定を読み直して agent:updated を発行する（項目の更新のあと用）。
func (s *Service) emitUpdatedNow(ctx context.Context) {
	cfg, err := s.loadSettings(ctx)
	if err != nil {
		s.log.Warn("agent: load settings failed", "kind", errKind(err))
		return
	}
	s.emitUpdated(ctx, cfg)
}

// OpenPanel はメインウィンドウを前面に出し、エージェントのパネルを開かせる（マスコットの押下）。
func (s *Service) OpenPanel(context.Context) error {
	if s.d.ShowMainWindow != nil {
		s.d.ShowMainWindow()
	}
	s.d.Events.Emit(events.AgentOpenPanel, nil)
	return nil
}
