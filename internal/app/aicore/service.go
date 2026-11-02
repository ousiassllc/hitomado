// Package aicore は AI 設定・同意・返信下書き生成の実装（Service の本体）。
package aicore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

// AI プロバイダ名（API §6 の AIProvider）。
const (
	providerAnthropic = "anthropic"
	providerOpenAI    = "openai"
)

// aiGenerateTimeout はプロバイダ呼び出しのタイムアウト（API §6）。
const aiGenerateTimeout = 60 * time.Second

// SessionLookup は返信系セッションの種別と元スレッドの参照（composecore.Service が実装する）。
type SessionLookup interface {
	ThreadRef(sessionID string) (model.Mode, dto.ThreadRef, error)
}

// Service は AI 設定・同意・下書き生成（バインディング仕様 §6）。
type Service struct {
	d        *deps.Deps
	sessions SessionLookup

	timeout time.Duration

	mu       sync.Mutex
	inflight map[string]context.CancelCauseFunc // requestID → cancel
}

// New は Service を作る。返信系セッションの参照に sessions を使う。
func New(d *deps.Deps, sessions SessionLookup) *Service {
	return &Service{d: d, sessions: sessions, timeout: aiGenerateTimeout, inflight: map[string]context.CancelCauseFunc{}}
}

// storedAIConfig は app_settings の ai.config（ER §3.9）。
type storedAIConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"baseUrl,omitempty"`
}

func validProvider(p string) bool { return p == providerAnthropic || p == providerOpenAI }

func (s *Service) loadConfig(ctx context.Context) (storedAIConfig, error) {
	cfg := storedAIConfig{Provider: providerOpenAI}
	v, ok, err := s.d.Store.GetSetting(ctx, store.SettingAIConfig)
	if err != nil {
		return cfg, apperr.FromInfra(err, "")
	}
	if ok {
		if err := json.Unmarshal([]byte(v), &cfg); err != nil {
			return storedAIConfig{Provider: providerOpenAI}, apperr.Wrap(apperr.Internal, "AI 設定を読み込めません。", err)
		}
	}
	if !validProvider(cfg.Provider) {
		cfg.Provider = providerOpenAI
	}
	return cfg, nil
}

// apiKey は資格情報ストアの API キー。未設定は ok=false。
func (s *Service) apiKey(provider string) (key string, ok bool, err error) {
	key, err = s.d.Secrets.Get(secrets.AIAPIKeyKey(provider))
	if errors.Is(err, secrets.ErrNotFound) || (err == nil && key == "") {
		return "", false, nil
	}
	if err != nil {
		return "", false, apperr.Wrap(apperr.Internal, "資格情報ストアから API キーを読み込めません。", err)
	}
	return key, true, nil
}

func (s *Service) config(ctx context.Context) (dto.AIConfig, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return dto.AIConfig{}, err
	}
	consented, err := s.loadConsent(ctx, cfg)
	if err != nil {
		return dto.AIConfig{}, err
	}
	_, has, err := s.apiKey(cfg.Provider)
	if err != nil {
		return dto.AIConfig{}, err
	}
	return dto.AIConfig{Provider: cfg.Provider, Model: cfg.Model, BaseURL: cfg.BaseURL, HasAPIKey: has, ConsentedProviders: consented}, nil
}

// GetAIConfig は AI 設定を返す（API キー自体は返さない）。
func (s *Service) GetAIConfig(ctx context.Context) (dto.AIConfig, error) { return s.config(ctx) }

func errBadProvider() error {
	return apperr.New(apperr.Validation, "AI プロバイダが正しくありません（anthropic または openai）。")
}

// SetAIConfig は AI 設定を更新する。baseUrl は openai のみ有効で、http(s) の URL を要する
// （userinfo・query・fragment は拒否。anthropic のときは無視して保存しない）。
func (s *Service) SetAIConfig(ctx context.Context, provider, model, baseURL string) (dto.AIConfig, error) {
	if !validProvider(provider) {
		return dto.AIConfig{}, errBadProvider()
	}
	baseURL = strings.TrimSpace(baseURL)
	if provider != providerOpenAI {
		baseURL = ""
	}
	if baseURL != "" {
		if err := validateBaseURL(baseURL); err != nil {
			return dto.AIConfig{}, err
		}
	}
	b, err := json.Marshal(storedAIConfig{Provider: provider, Model: strings.TrimSpace(model), BaseURL: baseURL})
	if err != nil {
		return dto.AIConfig{}, apperr.Wrap(apperr.Internal, "AI 設定を保存できません。", err)
	}
	if err := s.d.Store.SetSetting(ctx, store.SettingAIConfig, string(b)); err != nil {
		return dto.AIConfig{}, apperr.FromInfra(err, "")
	}
	// 送信先（Base URL のホスト）が変わったプロバイダの同意は無効にする。API キーは維持する。
	if err := s.pruneConsent(ctx, storedAIConfig{Provider: provider, Model: model, BaseURL: baseURL}); err != nil {
		return dto.AIConfig{}, err
	}
	return s.config(ctx)
}

// SetAIApiKey は API キーを資格情報ストアへ保存する（NF-S01）。キーはログに出さない。
func (s *Service) SetAIApiKey(ctx context.Context, provider, apiKey string) (dto.AIConfig, error) {
	if !validProvider(provider) {
		return dto.AIConfig{}, errBadProvider()
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return dto.AIConfig{}, apperr.New(apperr.Validation, "API キーを入力してください。")
	}
	if err := s.d.Secrets.Set(secrets.AIAPIKeyKey(provider), apiKey); err != nil {
		return dto.AIConfig{}, apperr.Wrap(apperr.Internal, "API キーを資格情報ストアへ保存できません。", err)
	}
	return s.config(ctx)
}

// ClearAIApiKey は API キーを削除する。
func (s *Service) ClearAIApiKey(ctx context.Context, provider string) (dto.AIConfig, error) {
	if !validProvider(provider) {
		return dto.AIConfig{}, errBadProvider()
	}
	if err := s.d.Secrets.Delete(secrets.AIAPIKeyKey(provider)); err != nil {
		return dto.AIConfig{}, apperr.Wrap(apperr.Internal, "API キーを削除できません。", err)
	}
	return s.config(ctx)
}
