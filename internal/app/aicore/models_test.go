package aicore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
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

func (f *aiFixture) withModels(m *modelsLister) { f.Deps.AIModels = m.fn }

func TestListAIModelsUsesSavedBaseURLAndKey(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{list: []ai.ModelInfo{{ID: "gpt-a"}, {ID: "claude-x", DisplayName: "Claude X"}}}
	f.withModels(m)
	f.ready(t) // openai + https://llm.example/v1 に同意・キー済み

	got, err := f.ai.ListAIModels(t.Context(), "openai")
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

	_, err := f.ai.SetAIConfig(t.Context(), "anthropic", "claude", "https://ignored.example")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "anthropic", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic")
	require.NoError(t, err)
	got, err := f.ai.ListAIModels(t.Context(), "anthropic")
	require.NoError(t, err)
	assert.Len(t, got.Models, 1)
	assert.Equal(t, "", m.calls[0].baseURL, "anthropic に Base URL は無い")
	assert.Equal(t, testAPIKey, m.calls[0].cred)

	f.chatgptReady(t)
	got, err = f.ai.ListAIModels(t.Context(), "chatgpt")
	require.NoError(t, err)
	assert.Len(t, got.Models, 1)
	assert.Equal(t, chatgptToken, m.calls[1].cred, "chatgpt はログインのアクセストークンで取得する")
}

func TestListAIModelsSendsNothingBeforeConsentOrWithoutCredentials(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{list: []ai.ModelInfo{{ID: "m1"}}}
	f.withModels(m)
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "", "https://llm.example/v1")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)

	got, err := f.ai.ListAIModels(t.Context(), "openai")
	require.NoError(t, err)
	assert.Equal(t, "consent_required", got.Reason)
	assert.Empty(t, got.Models)
	assert.NotNil(t, got.Models)
	assert.NotEmpty(t, got.Message)

	// 同意後でもキーが無ければ送らない。
	_, err = f.ai.RecordAIConsent(t.Context(), "openai")
	require.NoError(t, err)
	_, err = f.ai.ClearAIApiKey(t.Context(), "openai")
	require.NoError(t, err)
	got, err = f.ai.ListAIModels(t.Context(), "openai")
	require.NoError(t, err)
	assert.Equal(t, "no_credentials", got.Reason)

	// Base URL を変えると同意は外れ、送らない。
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.SetAIConfig(t.Context(), "openai", "", "https://other.example/v1")
	require.NoError(t, err)
	got, err = f.ai.ListAIModels(t.Context(), "openai")
	require.NoError(t, err)
	assert.Equal(t, "consent_required", got.Reason)

	// ChatGPT は、未ログインなら no_credentials。
	f.ChatGPT.Token = chatgptToken
	_, err = f.ai.RecordAIConsent(t.Context(), "chatgpt")
	require.NoError(t, err)
	got, err = f.ai.ListAIModels(t.Context(), "chatgpt")
	require.NoError(t, err)
	assert.Equal(t, "no_credentials", got.Reason)

	assert.Empty(t, m.calls, "同意前・キーなしでは一覧の取得を送らない")
}

func TestListAIModelsDoesNotSendToAnInvalidSavedBaseURL(t *testing.T) {
	f := newAIFixture(t)
	m := &modelsLister{list: []ai.ModelInfo{{ID: "m1"}}}
	f.withModels(m)
	f.ready(t)
	// 旧設定を模して、保存済みの Base URL を非 loopback の http:// にする（SetAIConfig は拒否するので直接書く）。
	require.NoError(t, f.Store.SetSetting(t.Context(), "ai.config",
		`{"provider":"openai","model":"gpt-test","baseUrl":"http://llm.example/v1"}`))
	got, err := f.ai.ListAIModels(t.Context(), "openai")
	require.NoError(t, err)
	assert.Equal(t, "invalid_base_url", got.Reason)
	assert.Empty(t, m.calls)
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
		got, err := f.ai.ListAIModels(t.Context(), "openai")
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
	_, err := f.ai.ListAIModels(t.Context(), "gemini")
	requireCode(t, "VALIDATION", err)

	ctx, cancel := context.WithCancel(t.Context())
	f.withModels(&modelsLister{})
	f.Deps.AIModels = func(ctx context.Context, _, _, _ string) ([]ai.ModelInfo, error) {
		cancel()
		return nil, ctx.Err()
	}
	_, err = f.ai.ListAIModels(ctx, "openai")
	requireCode(t, "CANCELLED", err)
}

func TestSavedModelAndConfigAreUntouchedByListing(t *testing.T) {
	f := newAIFixture(t)
	f.withModels(&modelsLister{list: []ai.ModelInfo{{ID: "other"}}})
	f.ready(t)
	_, err := f.ai.ListAIModels(t.Context(), "openai")
	require.NoError(t, err)
	cfg, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "gpt-test", cfg.Model, "保存済みのモデルが一覧に無くても消さない")
}
