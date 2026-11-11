// Package aicore は AI 設定・同意・返信下書き生成の実装（Service の本体）。
package aicore

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/ai"
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
	providerChatGPT   = "chatgpt" // Sign in with ChatGPT（利用者自身の ChatGPT プラン。Responses API）
	// providerTypeSafe は TypeSafe AI（Jev。#314）。文章を書かない分類用のモデルで、Inbox Zero の判定（FindInboxZeroCandidates）だけに使う。
	// 返信下書き・要約・質問には使えない（validProvider の外）。API キーと同意（件名・差出人・本文の先頭の送信）は持つ（validSendProvider）。
	providerTypeSafe = "typesafe"
)

// defaultProvider は AI の設定をまだ保存していないときのプロバイダ（利用者自身の ChatGPT プラン。#104）。
// 画面の既定もこの値に揃える（GetAIConfig の provider をそのまま使う）。保存済みのプロバイダは変えない。
const defaultProvider = providerChatGPT

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

// Option は Service の任意の設定。
type Option func(*Service)

// WithTimeout はプロバイダ呼び出しのタイムアウトを変える（既定は 60 秒。テストで短縮するのに使う）。
func WithTimeout(d time.Duration) Option { return func(s *Service) { s.timeout = d } }

// New は Service を作る。返信系セッションの参照に sessions を使う。
func New(d *deps.Deps, sessions SessionLookup, opts ...Option) *Service {
	s := &Service{d: d, sessions: sessions, timeout: aiGenerateTimeout, inflight: map[string]context.CancelCauseFunc{}}
	for _, o := range opts {
		o(s)
	}
	return s
}

// storedAIConfig は app_settings の ai.config（ER §3.9）。
type storedAIConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	BaseURL  string `json:"baseUrl,omitempty"`
	// Effort はモデルのエフォート（#111）。「モデルの既定」はキー自体を書かない。範囲外の保存値は loadConfigState が空にする。
	Effort string `json:"effort,omitempty"`
}

// validProvider は返信下書き・要約・質問に使えるプロバイダか（AI 設定の provider・モデル一覧）。
func validProvider(p string) bool {
	return p == providerAnthropic || p == providerOpenAI || p == providerChatGPT
}

// validSendProvider はメールの内容の送信先になり得るプロバイダ（API キーの登録・同意の対象）か。validProvider に TypeSafe AI を足したもの。
func validSendProvider(p string) bool {
	return validProvider(p) || p == providerTypeSafe
}

// loadConfig は AI 設定を返す。未保存（または保存値が壊れている・未知のプロバイダ）のときは defaultProvider の既定値。
func (s *Service) loadConfig(ctx context.Context) (storedAIConfig, error) {
	cfg, _, err := s.loadConfigState(ctx)
	return cfg, err
}

// loadConfigState は loadConfig に加えて、AI 設定を保存済みか（有効なプロバイダを持つ値が app_settings にあるか）を返す。
func (s *Service) loadConfigState(ctx context.Context) (cfg storedAIConfig, saved bool, err error) {
	cfg = storedAIConfig{Provider: defaultProvider}
	v, ok, err := s.d.Store.GetSetting(ctx, store.SettingAIConfig)
	if err != nil {
		return cfg, false, apperr.FromInfra(err, "")
	}
	if !ok {
		return cfg, false, nil
	}
	if err := json.Unmarshal([]byte(v), &cfg); err != nil {
		return storedAIConfig{Provider: defaultProvider}, false, apperr.Wrap(apperr.Internal, "AI 設定を読み込めません。", err)
	}
	if !validProvider(cfg.Provider) {
		return storedAIConfig{Provider: defaultProvider}, false, nil
	}
	// 範囲外の保存値（手で書き換えられた値）は無視して「モデルの既定」として扱い、プロバイダへ送らない。
	if !ai.ValidEffort(cfg.Provider, cfg.Effort) {
		cfg.Effort = ""
	}
	return cfg, true, nil
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
	cfg, configured, err := s.loadConfigState(ctx)
	if err != nil {
		return dto.AIConfig{}, err
	}
	consented, err := s.loadConsent(ctx, cfg)
	if err != nil {
		return dto.AIConfig{}, err
	}
	gpt, err := s.chatGPTStatus(ctx)
	if err != nil {
		return dto.AIConfig{}, err
	}
	keyed := []string{}
	for _, p := range []string{providerAnthropic, providerOpenAI, providerTypeSafe} {
		_, ok, err := s.apiKey(p)
		if err != nil {
			return dto.AIConfig{}, err
		}
		if ok {
			keyed = append(keyed, p)
		}
	}
	has := gpt.SignedIn
	if cfg.Provider != providerChatGPT {
		has = slices.Contains(keyed, cfg.Provider)
	}
	return dto.AIConfig{
		Provider: cfg.Provider, Model: cfg.Model, BaseURL: cfg.BaseURL, Effort: cfg.Effort, Configured: configured, HasAPIKey: has,
		APIKeyProviders: keyed, ConsentedProviders: consented,
		ChatGPTSignedIn: gpt.SignedIn, ChatGPTEmail: gpt.Email, ChatGPTAccounts: chatGPTAccounts(gpt),
	}, nil
}

// GetAIConfig は AI 設定を返す（API キー自体は返さない）。
func (s *Service) GetAIConfig(ctx context.Context) (dto.AIConfig, error) { return s.config(ctx) }

func errBadProvider() error {
	return apperr.New(apperr.Validation, "AI プロバイダが正しくありません（anthropic・openai・chatgpt のいずれか）。")
}

// errChatGPTNoKey は chatgpt に API キー操作が来たときのエラー（ChatGPT ではログインでトークンを得る）。
func errChatGPTNoKey() error {
	return apperr.New(apperr.Validation, "ChatGPT のプランは API キーを使いません。「Continue with ChatGPT」でログインしてください。")
}

// SetAIConfig は AI 設定を更新する。baseUrl は openai のみ有効で、http(s) の URL を要する
// （userinfo・query・fragment は拒否。anthropic のときは無視して保存しない）。
// effort（#111）は省略・空で「モデルの既定」（キーを書かない）。指定できるのはプロバイダごとの段階だけで、範囲外は VALIDATION。
// モデルとの組み合わせは確かめない（確かめるのは画面。生成時にプロバイダが拒否したら AI_FAILED）。渡さなければ前の値は引き継がず「既定」に戻る。
func (s *Service) SetAIConfig(ctx context.Context, provider, model, baseURL, effort string) (dto.AIConfig, error) {
	if !validProvider(provider) {
		return dto.AIConfig{}, errBadProvider()
	}
	effort = strings.TrimSpace(effort)
	if effort != "" && !ai.ValidEffort(provider, effort) {
		return dto.AIConfig{}, apperr.New(apperr.Validation, "エフォートが正しくありません。")
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
	b, err := json.Marshal(storedAIConfig{Provider: provider, Model: strings.TrimSpace(model), BaseURL: baseURL, Effort: effort})
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
	if !validSendProvider(provider) {
		return dto.AIConfig{}, errBadProvider()
	}
	if provider == providerChatGPT {
		return dto.AIConfig{}, errChatGPTNoKey()
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
	if !validSendProvider(provider) {
		return dto.AIConfig{}, errBadProvider()
	}
	if provider == providerChatGPT {
		return dto.AIConfig{}, errChatGPTNoKey()
	}
	if err := s.d.Secrets.Delete(secrets.AIAPIKeyKey(provider)); err != nil {
		return dto.AIConfig{}, apperr.Wrap(apperr.Internal, "API キーを削除できません。", err)
	}
	return s.config(ctx)
}
