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
		// リダイレクトは追従しない。307／308 は POST の本文ごと別ホスト・別 scheme へ再送されるため、
		// 同意・検証済みの送信先（scheme＋ホスト）以外にスレッド内容が届き得る。3xx は ErrFailed になる。
		hc = &http.Client{
			Timeout:       defaultHTTPTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
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

// Base URL の検証エラー。メッセージに URL は含めない（認証情報・クエリを漏らさない）。
var (
	// ErrInvalidBaseURL は Base URL が不正（http(s) 以外・host なし・userinfo・query・fragment を含む）なこと。
	ErrInvalidBaseURL = errors.New("ai: ベース URL が不正です")
	// ErrInsecureBaseURL は http:// の Base URL が loopback 以外を指すこと（ErrInvalidBaseURL でもある）。
	ErrInsecureBaseURL = fmt.Errorf("%w（http:// は localhost／127.0.0.1／::1 のときだけ使えます）", ErrInvalidBaseURL)
)

// ValidateBaseURL は openai 互換の Base URL を検証する（保存時と接続時で同じ規則を使う）。
//   - http／https 以外・host なし・userinfo・query・fragment を含む URL は ErrInvalidBaseURL。
//   - https:// は任意のホスト。http:// はホストが localhost／127.0.0.1／::1 に厳密一致するときだけ許可
//     （大文字小文字違い・末尾ドット・localhost.evil.com・0.0.0.0・127.1・IPv4-mapped IPv6 などは ErrInsecureBaseURL）。
//
// raw は空でないこと（空＝既定の Base URL は呼び出し側で扱う）。
func ValidateBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || strings.ContainsAny(raw, "?#") {
		return ErrInvalidBaseURL
	}
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		return ErrInsecureBaseURL
	}
	return nil
}

// isLoopbackHost は host が localhost／127.0.0.1／::1 に厳密一致するか。
func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// normalizeBase はベース URL を検証し、末尾の / を除く。空なら def。
func normalizeBase(raw, def string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def, nil
	}
	if err := ValidateBaseURL(raw); err != nil {
		return "", err
	}
	return strings.TrimRight(raw, "/"), nil
}
