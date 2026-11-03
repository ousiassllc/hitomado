package aicore

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/store"
)

// 既定の送信先（internal/ai の既定 Base URL と同じ scheme://ホスト）。
const (
	defaultAnthropicHost = "https://api.anthropic.com"
	defaultOpenAIHost    = "https://api.openai.com"
	// chatgpt の送信先（Responses API）。ログインは別ホスト（auth.openai.com）だが、スレッド内容を送るのはここだけ。
	defaultChatGPTHost = "https://api.openai.com"
)

// consentEntry は同意 1 件。同意は「プロバイダ＋送信先（Base URL の scheme とホスト）」単位。
// BaseURL が空は、そのプロバイダの既定の送信先（旧形式の同意もこれとして読む）。
type consentEntry struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl,omitempty"`
	AgreedAt int64  `json:"agreedAt"`
}

// storedAIConsent は app_settings の ai.consent。複数プロバイダ分を Providers に持つ。
// Provider／AgreedAt は旧形式（単一プロバイダのオブジェクト）の読み取り専用。
type storedAIConsent struct {
	Provider  string         `json:"provider,omitempty"`
	AgreedAt  int64          `json:"agreedAt,omitempty"`
	Providers []consentEntry `json:"providers,omitempty"`
}

// validateBaseURL は Base URL を検証する（ai.ValidateBaseURL と同じ規則。URL はメッセージに含めない）。
func validateBaseURL(raw string) error {
	err := ai.ValidateBaseURL(raw)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ai.ErrInsecureBaseURL):
		return apperr.Wrap(apperr.Validation, "http:// は localhost／127.0.0.1／::1 のときだけ使えます。", err)
	default:
		return apperr.Wrap(apperr.Validation, "Base URL は http:// または https:// で始まる、認証情報・クエリ・フラグメントを含まない URL を指定してください。", err)
	}
}

// hostKey は Base URL の送信先（scheme://ホスト。小文字・既定ポートと末尾のドットを除く。パスは含めない）。
// 空は既定の送信先 def。scheme を含めるので https→http の変更（平文化）も別の送信先になる。
func hostKey(raw, def string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "invalid:" + raw
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	return u.Scheme + "://" + host
}

// destination は同意の照合キー（プロバイダ＋送信先 scheme://ホスト）。anthropic に Base URL は無い。
func destination(provider, baseURL string) string {
	switch provider {
	case providerOpenAI:
		return provider + "|" + hostKey(baseURL, defaultOpenAIHost)
	case providerChatGPT:
		return provider + "|" + defaultChatGPTHost
	}
	return provider + "|" + defaultAnthropicHost
}

// currentBaseURL は現在の設定での provider の Base URL（Base URL を持つのは openai のみ）。
func currentBaseURL(cfg storedAIConfig, provider string) string {
	if provider == providerOpenAI {
		return cfg.BaseURL
	}
	return ""
}

// valid は同意が現在の設定の送信先に対するものか。
// 現在の Base URL が使えない（保存済みの旧設定の非 loopback http:// など）ときは無効とする。
func (e consentEntry) valid(cfg storedAIConfig) bool {
	if !validProvider(e.Provider) {
		return false
	}
	cur := currentBaseURL(cfg, e.Provider)
	if cur != "" && ai.ValidateBaseURL(cur) != nil {
		return false
	}
	return destination(e.Provider, e.BaseURL) == destination(e.Provider, cur)
}

// loadConsentEntries は保存済みの同意（プロバイダごとに 1 件・重複なし）を返す。
func (s *Service) loadConsentEntries(ctx context.Context) ([]consentEntry, error) {
	out := []consentEntry{}
	v, ok, err := s.d.Store.GetSetting(ctx, store.SettingAIConsent)
	if err != nil {
		return out, apperr.FromInfra(err, "")
	}
	if !ok {
		return out, nil
	}
	var c storedAIConsent
	if err := json.Unmarshal([]byte(v), &c); err != nil {
		return out, apperr.Wrap(apperr.Internal, "AI の同意状態を読み込めません。", err)
	}
	for _, e := range append([]consentEntry{{Provider: c.Provider, AgreedAt: c.AgreedAt}}, c.Providers...) {
		if validProvider(e.Provider) && !slices.ContainsFunc(out, func(o consentEntry) bool { return o.Provider == e.Provider }) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *Service) saveConsentEntries(ctx context.Context, entries []consentEntry) error {
	b, err := json.Marshal(storedAIConsent{Providers: entries})
	if err != nil {
		return apperr.Wrap(apperr.Internal, "同意を保存できません。", err)
	}
	if err := s.d.Store.SetSetting(ctx, store.SettingAIConsent, string(b)); err != nil {
		return apperr.FromInfra(err, "")
	}
	return nil
}

// loadConsent は現在の設定（送信先）に対して有効な同意済みプロバイダを返す。
func (s *Service) loadConsent(ctx context.Context, cfg storedAIConfig) ([]string, error) {
	entries, err := s.loadConsentEntries(ctx)
	out := []string{}
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		if e.valid(cfg) {
			out = append(out, e.Provider)
		}
	}
	return out, nil
}

// pruneConsent は現在の設定の送信先と合わない同意（Base URL が変わったもの）を保存から外す。
func (s *Service) pruneConsent(ctx context.Context, cfg storedAIConfig) error {
	entries, err := s.loadConsentEntries(ctx)
	if err != nil {
		return err
	}
	kept := slices.DeleteFunc(slices.Clone(entries), func(e consentEntry) bool { return !e.valid(cfg) })
	if len(kept) == len(entries) {
		return nil
	}
	return s.saveConsentEntries(ctx, kept)
}

// RecordAIConsent は「スレッド内容が当該プロバイダ（現在の送信先）へ送信される」ことへの同意を保存する（F-08）。
func (s *Service) RecordAIConsent(ctx context.Context, provider string) (dto.AIConfig, error) {
	if !validProvider(provider) {
		return dto.AIConfig{}, errBadProvider()
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return dto.AIConfig{}, err
	}
	entries, err := s.loadConsentEntries(ctx)
	if err != nil {
		return dto.AIConfig{}, err
	}
	at := s.d.Clock.Now().UnixMilli()
	for _, e := range entries {
		if e.Provider == provider && e.valid(cfg) {
			at = e.AgreedAt
		}
	}
	entries = slices.DeleteFunc(entries, func(e consentEntry) bool { return e.Provider == provider })
	entries = append(entries, consentEntry{Provider: provider, BaseURL: currentBaseURL(cfg, provider), AgreedAt: at})
	if err := s.saveConsentEntries(ctx, entries); err != nil {
		return dto.AIConfig{}, err
	}
	return s.config(ctx)
}
