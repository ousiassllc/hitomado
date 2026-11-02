package ai

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// プロバイダ名（AIProvider。docs/api §6）。
const (
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
)

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	defaultOpenAIBaseURL    = "https://api.openai.com/v1"
	anthropicVersion        = "2023-06-01"
	anthropicMaxTokens      = 2048
	// defaultHTTPTimeout は ctx が無期限だった場合の保険。実際の期限は呼び出し側の ctx（60 秒）。
	defaultHTTPTimeout = 2 * time.Minute
)

// New は provider（anthropic／openai）に応じた Provider を返す。baseURL は openai（互換エンドポイント）のみ
// 有効で、空なら OpenAI 既定。anthropic では無視される。
func New(provider, model, baseURL, apiKey string) (Provider, error) {
	if provider == ProviderAnthropic {
		baseURL = ""
	}
	return NewWithClient(provider, model, baseURL, apiKey, nil)
}

// NewWithClient は http.Client とベース URL を指定して Provider を作る（テスト用に anthropic の
// ベース URL も上書きできる）。hc が nil なら既定のクライアントを使う。
func NewWithClient(provider, model, baseURL, apiKey string, hc *http.Client) (Provider, error) {
	if model == "" {
		return nil, errors.New("ai: モデルが指定されていません")
	}
	if hc == nil {
		hc = &http.Client{Timeout: defaultHTTPTimeout}
	}
	c := client{hc: hc, model: model, apiKey: apiKey}
	switch provider {
	case ProviderAnthropic:
		base, err := normalizeBase(baseURL, defaultAnthropicBaseURL)
		if err != nil {
			return nil, err
		}
		c.base = base
		return &anthropic{client: c}, nil
	case ProviderOpenAI:
		base, err := normalizeBase(baseURL, defaultOpenAIBaseURL)
		if err != nil {
			return nil, err
		}
		c.base = base
		return &openAI{client: c}, nil
	default:
		return nil, fmt.Errorf("ai: 未対応のプロバイダです: %q", provider)
	}
}

// normalizeBase はベース URL を検証し、末尾の / を除く。空なら def。
// http(s) 以外・host なし・userinfo・query・fragment を含む URL は拒否する。
func normalizeBase(raw, def string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	u, err := url.Parse(raw)
	// 認証情報・クエリ・フラグメントは拒否する（API キーを URL に埋め込ませない）。
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || strings.ContainsAny(raw, "?#") {
		return "", errors.New("ai: ベース URL が不正です")
	}
	return strings.TrimRight(raw, "/"), nil
}
