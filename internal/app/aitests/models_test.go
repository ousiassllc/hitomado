package aitests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// --- モデル一覧（#76） ---

// modelsLister は deps.AIModelsLister のフェイク。呼ばれた引数を記録する。
type modelsLister struct {
	mu    sync.Mutex
	calls []struct{ provider, baseURL, cred string }
	list  []ai.ModelInfo
	err   error
}

func (m *modelsLister) fn(_ context.Context, provider, baseURL, cred string) ([]ai.ModelInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, struct{ provider, baseURL, cred string }{provider, baseURL, cred})
	return m.list, m.err
}

// readyBaseURL は f.ready が設定・同意する OpenAI 互換の Base URL。
const readyBaseURL = "https://llm.example/v1"

func (f *aiFixture) withModels(m *modelsLister) { f.Deps.AIModels = m.fn }

func TestListAIModelsUsesGivenBaseURLAndKey(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{list: []ai.ModelInfo{{ID: "gpt-a"}, {ID: "claude-x", DisplayName: "Claude X"}}}
	f.withModels(m)
	f.ready(t) // openai + https://llm.example/v1 に同意・キー済み

	got, err := f.ai.ListAIModels(t.Context(), "openai", readyBaseURL)
	require.NoError(t, err)
	assert.Empty(t, got.Reason)
	assert.Equal(t, "openai", got.Provider)
	require.Len(t, got.Models, 2)
	assert.Equal(t, "Claude X", got.Models[1].DisplayName)
	require.Len(t, m.calls, 1)
	assert.Equal(t, "https://llm.example/v1", m.calls[0].baseURL)
	assert.Equal(t, testAPIKey, m.calls[0].cred, "キーは資格情報ストアから読む")

	b, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(b), testAPIKey)
	assert.Contains(t, f.logs.String(), "count=2", "ログには件数だけ")
	assert.NotContains(t, f.logs.String(), "claude-x")
	assert.NotContains(t, f.logs.String(), testAPIKey)
}

func TestListAIModelsAnthropicAndChatGPT(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{list: []ai.ModelInfo{{ID: "m1"}}}
	f.withModels(m)

	_, err := f.ai.SetAIConfig(t.Context(), "anthropic", "claude", "https://ignored.example", "")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "anthropic", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)
	got, err := f.ai.ListAIModels(t.Context(), "anthropic", "")
	require.NoError(t, err)
	assert.Len(t, got.Models, 1)
	assert.Equal(t, "", m.calls[0].baseURL, "anthropic に Base URL は無い")
	assert.Equal(t, testAPIKey, m.calls[0].cred)

	f.chatgptReady(t)
	got, err = f.ai.ListAIModels(t.Context(), "chatgpt", "")
	require.NoError(t, err)
	assert.Len(t, got.Models, 1)
	assert.Equal(t, chatgptToken, m.calls[1].cred, "chatgpt はログインのアクセストークンで取得する")
}

func TestListAIModelsSendsNothingBeforeConsentOrWithoutCredentials(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{list: []ai.ModelInfo{{ID: "m1"}}}
	f.withModels(m)
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "", readyBaseURL, "")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)

	got, err := f.ai.ListAIModels(t.Context(), "openai", readyBaseURL)
	require.NoError(t, err)
	assert.Equal(t, "consent_required", got.Reason)
	assert.Empty(t, got.Models)
	assert.NotNil(t, got.Models)
	assert.NotEmpty(t, got.Message)

	// 同意後でもキーが無ければ送らない。
	_, err = f.ai.RecordAIConsent(t.Context(), "openai", readyBaseURL)
	require.NoError(t, err)
	_, err = f.ai.ClearAIApiKey(t.Context(), "openai")
	require.NoError(t, err)
	got, err = f.ai.ListAIModels(t.Context(), "openai", readyBaseURL)
	require.NoError(t, err)
	assert.Equal(t, "no_credentials", got.Reason)

	// 別の送信先（Base URL のホストが違う）は同意していないので、送らない。
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	got, err = f.ai.ListAIModels(t.Context(), "openai", "https://other.example/v1")
	require.NoError(t, err)
	assert.Equal(t, "consent_required", got.Reason)

	// ChatGPT は、同意済みで未ログインなら no_credentials。
	_, err = f.ai.RecordAIConsent(t.Context(), "chatgpt", "")
	require.NoError(t, err)
	got, err = f.ai.ListAIModels(t.Context(), "chatgpt", "")
	require.NoError(t, err)
	assert.Equal(t, "no_credentials", got.Reason)

	assert.Empty(t, m.calls, "同意前・キーなしでは一覧の取得を送らない")
}

func TestListAIModelsDoesNotSendToAnInvalidBaseURL(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{list: []ai.ModelInfo{{ID: "m1"}}}
	f.withModels(m)
	f.ready(t)
	// 非 loopback の http:// は、同意の保存があっても送らない（旧設定の保存値を模して直接書く）。
	require.NoError(t, f.Store.SetSetting(t.Context(), "ai.consent",
		`{"providers":[{"provider":"openai","baseUrl":"http://llm.example/v1","agreedAt":1}]}`))
	got, err := f.ai.ListAIModels(t.Context(), "openai", "http://llm.example/v1")
	require.NoError(t, err)
	assert.Equal(t, "invalid_base_url", got.Reason)
	assert.Empty(t, m.calls)
}

// 保存前（設定が無い）でも、選んだプロバイダの送信先に同意すれば、そのまま一覧を取得できる（#105）。
func TestListAIModelsBeforeSavingTheConfig(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{list: []ai.ModelInfo{{ID: "claude-x"}}}
	f.withModels(m)
	_, err := f.ai.SetAIApiKey(t.Context(), "anthropic", testAPIKey)
	require.NoError(t, err)

	// まだ何も保存していない。同意前は送らない。
	got, err := f.ai.ListAIModels(t.Context(), "anthropic", "")
	require.NoError(t, err)
	assert.Equal(t, "consent_required", got.Reason)
	assert.Empty(t, m.calls, "同意前の送信先には取得の通信を送らない")

	// 同意すると、設定を保存せずに一覧が取れる。
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)
	got, err = f.ai.ListAIModels(t.Context(), "anthropic", "")
	require.NoError(t, err)
	assert.Empty(t, got.Reason)
	require.Len(t, got.Models, 1)
	require.Len(t, m.calls, 1)
	assert.Equal(t, testAPIKey, m.calls[0].cred)

	cfg, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.False(t, cfg.Configured, "一覧の取得と同意では設定は保存されない")
	assert.Equal(t, "chatgpt", cfg.Provider)
}

// 保存前に選んだ OpenAI 互換の Base URL への同意は、その送信先にだけ効き、保存済みの送信先の同意を消さない。
func TestConsentForADraftDestinationKeepsTheSavedOne(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{list: []ai.ModelInfo{{ID: "m1"}}}
	f.withModels(m)
	f.ready(t) // 保存済み: openai + readyBaseURL に同意・キーあり

	got, err := f.ai.ListAIModels(t.Context(), "openai", "https://draft.example/v1")
	require.NoError(t, err)
	assert.Equal(t, "consent_required", got.Reason)
	assert.Empty(t, m.calls)

	cfg, err := f.ai.RecordAIConsent(t.Context(), "openai", "https://draft.example/v1")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, cfg.ConsentedProviders, "保存済みの送信先の同意は残る")
	got, err = f.ai.ListAIModels(t.Context(), "openai", "https://draft.example/v1")
	require.NoError(t, err)
	assert.Empty(t, got.Reason)
	got, err = f.ai.ListAIModels(t.Context(), "openai", readyBaseURL)
	require.NoError(t, err)
	assert.Empty(t, got.Reason, "保存済みの送信先に戻しても同意のまま")
	require.Len(t, m.calls, 2)
	assert.Equal(t, "https://draft.example/v1", m.calls[0].baseURL)
	assert.Equal(t, readyBaseURL, m.calls[1].baseURL)

	_, err = f.ai.RecordAIConsent(t.Context(), "openai", "http://llm.example/v1")
	requireCode(t, "VALIDATION", err)
}

func TestListAIModelsFallbackReasonsHideSecrets(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{}
	f.withModels(m)
	f.ready(t)
	cases := []struct {
		err    error
		reason string
	}{
		{ai.ErrModelsUnsupported, "unsupported"},
		{ai.ErrUnauthorized, "unauthorized"},
		{ai.ErrRateLimited, "rate_limited"},
		{&net.DNSError{Err: "no such host"}, "network"},
		{context.DeadlineExceeded, "network"},
		{ai.ErrFailed, "failed"},
		{errors.New("something"), "failed"},
	}
	for _, c := range cases {
		m.err = fmt.Errorf("%w: key=%s body=%s", c.err, testAPIKey, "RESPONSE-BODY-TEXT")
		got, err := f.ai.ListAIModels(t.Context(), "openai", readyBaseURL)
		require.NoError(t, err, c.reason)
		assert.Equal(t, c.reason, got.Reason)
		assert.Empty(t, got.Models)
		assert.NotEmpty(t, got.Message)
		b, _ := json.Marshal(got)
		assert.NotContains(t, string(b), testAPIKey)
		assert.NotContains(t, string(b), "RESPONSE-BODY-TEXT")
	}
	logged := f.logs.String()
	assert.NotContains(t, logged, testAPIKey)
	assert.NotContains(t, logged, "RESPONSE-BODY-TEXT")
	assert.Contains(t, logged, "reason=unsupported")
}

func TestListAIModelsCancelledAndValidation(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	_, err := f.ai.ListAIModels(t.Context(), "gemini", "")
	requireCode(t, "VALIDATION", err)

	ctx, cancel := context.WithCancel(t.Context())
	f.withModels(&modelsLister{})
	f.Deps.AIModels = func(ctx context.Context, _, _, _ string) ([]ai.ModelInfo, error) {
		cancel()
		return nil, ctx.Err()
	}
	_, err = f.ai.ListAIModels(ctx, "openai", readyBaseURL)
	requireCode(t, "CANCELLED", err)
}

func TestSavedModelAndConfigAreUntouchedByListing(t *testing.T) {
	f := newAIFixture(t)
	f.withModels(&modelsLister{list: []ai.ModelInfo{{ID: "other"}}})
	f.ready(t)
	_, err := f.ai.ListAIModels(t.Context(), "openai", readyBaseURL)
	require.NoError(t, err)
	cfg, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "gpt-test", cfg.Model, "保存済みのモデルが一覧に無くても消さない")
}

// Anthropic のモデルごとのエフォートの段階は、そのまま画面へ返す（対応しないモデルは省略。#111）。
func TestListAIModelsReturnsEfforts(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{list: []ai.ModelInfo{
		{ID: "claude-new", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
		{ID: "claude-old"},
	}}
	f.withModels(m)
	_, err := f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "anthropic", testAPIKey)
	require.NoError(t, err)

	got, err := f.ai.ListAIModels(t.Context(), "anthropic", "")
	require.NoError(t, err)
	require.Len(t, got.Models, 2)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, got.Models[0].Efforts)
	assert.Nil(t, got.Models[1].Efforts)

	b, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"efforts":["low","medium","high","xhigh","max"]`)
	assert.Equal(t, 1, strings.Count(string(b), "efforts"), "対応しないモデルは efforts を省く")
}
