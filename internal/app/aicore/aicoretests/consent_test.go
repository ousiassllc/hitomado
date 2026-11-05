package aicoretests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/store"
)

func (f *aiFixture) consented(t *testing.T) []string {
	t.Helper()
	cfg, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	return cfg.ConsentedProviders
}

func (f *aiFixture) generate(t *testing.T, sessionID string) error {
	t.Helper()
	_, err := f.ai.GenerateReplyDraft(t.Context(), sessionID, "r1", "")
	return err
}

func TestBaseURLRejectsUserinfoQueryFragment(t *testing.T) {
	f := newAIFixture(t)
	for _, u := range []string{
		"https://user:pass@llm.example/v1",
		"https://user@llm.example/v1",
		"https://llm.example/v1?key=secret",
		"https://llm.example/v1?",
		"https://llm.example/v1#frag",
		"https://llm.example/v1#",
	} {
		_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", u, "")
		ae := requireError(t, "VALIDATION", err)
		assert.NotContains(t, ae.Message, "secret")
		assert.NotContains(t, ae.Message, "pass")
	}
	for _, u := range []string{"http://localhost:1234/v1", "https://llm.example/v1/"} {
		_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", u, "")
		require.NoError(t, err, u)
	}
}

func TestConsentIsInvalidatedWhenBaseURLHostChanges(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t) // openai + https://llm.example/v1 に同意済み
	s := f.open(t, "reply")
	require.NoError(t, f.generate(t, s.SessionID))
	assert.Equal(t, []string{"openai"}, f.consented(t))

	// 同じ送信先（パス・大文字小文字・既定ポート違い）なら同意は維持される。
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://LLM.example:443/v2/", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, f.consented(t))

	// ホストが変わると同意は無効（保存からも外れる）。API キーは維持される。
	cfg, err := f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://other.example/v1", "")
	require.NoError(t, err)
	assert.Empty(t, cfg.ConsentedProviders)
	assert.True(t, cfg.HasAPIKey)
	requireCode(t, "AI_CONSENT_REQUIRED", f.generate(t, s.SessionID))
	v, ok, err := f.Store.GetSetting(t.Context(), store.SettingAIConsent)
	require.NoError(t, err)
	require.True(t, ok)
	assert.NotContains(t, v, "openai")

	// 元のホストに戻しても、改めて同意するまでは有効にならない。
	_, err = f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	assert.Empty(t, f.consented(t))
	requireCode(t, "AI_CONSENT_REQUIRED", f.generate(t, s.SessionID))

	cfg, err = f.ai.RecordAIConsent(t.Context(), "openai", "https://llm.example/v1")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, cfg.ConsentedProviders)
	require.NoError(t, f.generate(t, s.SessionID))
}

func TestConsentIsInvalidatedWhenSchemeDowngradesToHTTP(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://localhost:8443/v1", "")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	cfg, err := f.ai.RecordAIConsent(t.Context(), "openai", "https://localhost:8443/v1")
	require.NoError(t, err)
	require.Equal(t, []string{"openai"}, cfg.ConsentedProviders)
	s := f.open(t, "reply")
	require.NoError(t, f.generate(t, s.SessionID))

	cfg, err = f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "http://localhost:8443/v1", "")
	require.NoError(t, err)
	assert.Empty(t, cfg.ConsentedProviders, "https→http では同意は失効する")
	requireCode(t, "AI_CONSENT_REQUIRED", f.generate(t, s.SessionID))
}

func TestConsentIsPerDestinationHost(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", "https://a.example/v1", "")
	require.NoError(t, err)
	cfg, err := f.ai.RecordAIConsent(t.Context(), "openai", "https://a.example/v1")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, cfg.ConsentedProviders)

	_, err = f.ai.SetAIConfig(t.Context(), "openai", "m", "", "")
	require.NoError(t, err)
	assert.Empty(t, f.consented(t), "OpenAI 既定の送信先は a.example とは別")
}

func TestLegacyConsentIsTheDefaultDestination(t *testing.T) {
	f := newAIFixture(t)
	require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAIConsent, `{"provider":"openai","agreedAt":1730000000000}`))
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", "", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, f.consented(t), "旧形式は openai の既定 URL への同意")

	_, err = f.ai.SetAIConfig(t.Context(), "openai", "m", "https://llm.example/v1", "")
	require.NoError(t, err)
	assert.Empty(t, f.consented(t), "旧形式の同意はカスタム Base URL には効かない")
}

func TestConsentDroppedWhenSwitchingProviderClearsBaseURL(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", "https://llm.example/v1", "")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "openai", "https://llm.example/v1")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)

	cfg, err := f.ai.SetAIConfig(t.Context(), "anthropic", "claude", "https://ignored.example", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"anthropic"}, cfg.ConsentedProviders, "anthropic の同意は維持。openai は送信先が変わるため無効")

	cfg, err = f.ai.SetAIConfig(t.Context(), "openai", "m", "https://llm.example/v1", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"anthropic"}, cfg.ConsentedProviders, "openai は再同意が必要。anthropic の同意は残るが現在の設定では未使用")
}

func TestConsentTimestampKeptOnRerecord(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.RecordAIConsent(t.Context(), "openai", "")
	require.NoError(t, err)
	v1, _, err := f.Store.GetSetting(t.Context(), store.SettingAIConsent)
	require.NoError(t, err)
	f.Clock.Advance(3_600_000_000_000)
	_, err = f.ai.RecordAIConsent(t.Context(), "openai", "")
	require.NoError(t, err)
	v2, _, err := f.Store.GetSetting(t.Context(), store.SettingAIConsent)
	require.NoError(t, err)
	assert.Equal(t, v1, v2)
}

func TestBaseURLHTTPOnlyForLoopbackOnSet(t *testing.T) {
	f := newAIFixture(t)
	for _, u := range []string{"http://localhost", "http://127.0.0.1:11434/v1", "http://[::1]:8080", "https://llm.example/v1"} {
		_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", u, "")
		require.NoError(t, err, u)
	}
	for _, u := range []string{
		"http://llm.example/v1", "http://192.168.0.5:11434/v1", "http://localhost.evil.com/v1",
		"http://127.0.0.1.evil.com", "http://0.0.0.0:8080", "http://127.1", "http://[::ffff:127.0.0.1]:8080",
		"http://localhost./v1", "http://user:pass@localhost:8080/v1",
	} {
		_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", u, "")
		ae := requireError(t, "VALIDATION", err)
		assert.NotContains(t, ae.Message, "evil")
		assert.NotContains(t, ae.Message, "pass")
		assert.NotContains(t, ae.Message, "192.168")
	}
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", "http://llm.example/v1", "")
	ae := requireError(t, "VALIDATION", err)
	assert.Equal(t, "http:// は localhost／127.0.0.1／::1 のときだけ使えます。", ae.Message)
	_, err = f.ai.SetAIConfig(t.Context(), "anthropic", "m", "http://llm.example/v1", "")
	require.NoError(t, err, "anthropic では Base URL を無視する")
}

func TestSavedInsecureBaseURLIsRejectedBeforeConnecting(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	s := f.open(t, "reply")
	// 旧バージョンが保存した非 loopback の http:// 設定（同意は旧形式で保存済み）。
	require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAIConfig,
		`{"provider":"openai","model":"gpt-test","baseUrl":"http://llm.example/v1"}`))
	require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAIConsent,
		`{"providers":[{"provider":"openai","baseUrl":"http://llm.example/v1","agreedAt":1}]}`))

	assert.Empty(t, f.consented(t), "使えない Base URL への同意は有効にしない")
	f.factory.provider = ""
	ae := requireError(t, "VALIDATION", f.generate(t, s.SessionID))
	assert.NotContains(t, ae.Message, "llm.example")
	assert.Empty(t, f.factory.provider, "プロバイダを初期化（接続）しない")
	assert.Empty(t, f.provider.inputs)
}

func TestFactoryInvalidBaseURLIsValidation(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.Deps.AI = func(string, string, string, string) (ai.Provider, error) { return nil, ai.ErrInsecureBaseURL }
	s := f.open(t, "reply")
	requireCode(t, "VALIDATION", f.generate(t, s.SessionID))
}
