package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// ModelInfo は選択可能なモデル。DisplayName はプロバイダが返さない場合は空。
type ModelInfo struct {
	ID          string
	DisplayName string
}

// ErrModelsUnsupported はプロバイダにモデル一覧 API が無いこと（404／405／501）。
var ErrModelsUnsupported = errors.New("ai: models api unsupported")

// anthropicModelsMaxPages は Anthropic のモデル一覧で辿るページ数の上限。
const anthropicModelsMaxPages = 5

// ListModels は provider のモデル一覧を返す。空なら非 nil の空スライス。重複・空 ID は除く。
//   - openai: GET {base}/models（Base URL は New と同じ検証。不正なら ErrInvalidBaseURL／ErrInsecureBaseURL で、通信しない）
//   - anthropic: GET /v1/models?limit=1000（has_more の間 after_id で最大 5 ページ）
//   - chatgpt: GET {base}/models（visibility が "list" のものだけ、サーバー順）
//
// baseURL は openai では常に有効。anthropic・chatgpt では NewWithClient と同じく hc が非 nil（テスト）のときだけ有効で、
// hc が nil なら既定の接続先を使う。hc が nil なら、リダイレクトを追従しない既定のクライアントを使う。
// エラーは New／GenerateReply と同じ分類（ErrUnauthorized／ErrRateLimited／ErrFailed）に加え、
// API が無いときは ErrModelsUnsupported。本文・キー・URL はエラーに含めない。
func ListModels(ctx context.Context, provider, baseURL, apiKey string, hc *http.Client) ([]ModelInfo, error) {
	switch provider {
	case ProviderOpenAI, ProviderAnthropic, ProviderChatGPT:
	default:
		return nil, fmt.Errorf("ai: 未対応のプロバイダです: %q", provider)
	}
	if provider != ProviderOpenAI && hc == nil {
		baseURL = ""
	}
	def := defaultOpenAIBaseURL
	if provider == ProviderAnthropic {
		def = defaultAnthropicBaseURL
	}
	base, err := normalizeBase(baseURL, def)
	if err != nil {
		return nil, err
	}
	if hc == nil {
		hc = newHTTPClient()
	}
	c := client{hc: hc, base: base, apiKey: apiKey}

	switch provider {
	case ProviderAnthropic:
		return c.listAnthropicModels(ctx)
	case ProviderChatGPT:
		return c.listChatGPTModels(ctx)
	default:
		return c.listOpenAIModels(ctx)
	}
}

// modelSet は順序を保ち、空 ID と重複を除いてモデルを集める。
type modelSet struct {
	list []ModelInfo
	seen map[string]struct{}
}

func newModelSet() *modelSet {
	return &modelSet{list: []ModelInfo{}, seen: map[string]struct{}{}}
}

func (s *modelSet) add(id, displayName string) {
	if id == "" {
		return
	}
	if _, dup := s.seen[id]; dup {
		return
	}
	s.seen[id] = struct{}{}
	s.list = append(s.list, ModelInfo{ID: id, DisplayName: displayName})
}

func (c client) listOpenAIModels(ctx context.Context) ([]ModelInfo, error) {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	headers := map[string]string{"Authorization": "Bearer " + c.apiKey}
	if err := c.getJSON(ctx, c.base+"/models", headers, &resp); err != nil {
		return nil, err
	}
	set := newModelSet()
	for _, m := range resp.Data {
		set.add(m.ID, "")
	}
	return set.list, nil
}

func (c client) listChatGPTModels(ctx context.Context) ([]ModelInfo, error) {
	var resp struct {
		Models []struct {
			Slug        string `json:"slug"`
			DisplayName string `json:"display_name"`
			Visibility  string `json:"visibility"`
		} `json:"models"`
	}
	headers := map[string]string{"Authorization": "Bearer " + c.apiKey}
	if err := c.getJSON(ctx, c.base+"/models", headers, &resp); err != nil {
		return nil, err
	}
	set := newModelSet()
	for _, m := range resp.Models {
		if m.Visibility == "list" {
			set.add(m.Slug, m.DisplayName)
		}
	}
	return set.list, nil
}

func (c client) listAnthropicModels(ctx context.Context) ([]ModelInfo, error) {
	headers := map[string]string{"x-api-key": c.apiKey, "anthropic-version": anthropicVersion}
	set := newModelSet()
	endpoint := c.base + "/v1/models?limit=1000"
	for range anthropicModelsMaxPages {
		var resp struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := c.getJSON(ctx, endpoint, headers, &resp); err != nil {
			return nil, err
		}
		for _, m := range resp.Data {
			set.add(m.ID, m.DisplayName)
		}
		if !resp.HasMore || resp.LastID == "" {
			break
		}
		endpoint = c.base + "/v1/models?limit=1000&after_id=" + url.QueryEscape(resp.LastID)
	}
	return set.list, nil
}
