package aicore_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	_, err := f.ai.GenerateReplyDraft(t.Context(), sessionID, "r1")
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
		_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", u)
		ae := requireError(t, "VALIDATION", err)
		assert.NotContains(t, ae.Message, "secret")
		assert.NotContains(t, ae.Message, "pass")
	}
	for _, u := range []string{"http://localhost:1234/v1", "https://llm.example/v1/"} {
		_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", u)
		require.NoError(t, err, u)
	}
}

func TestConsentIsInvalidatedWhenBaseURLHostChanges(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t) // openai + https://llm.example/v1 に同意済み
	s := f.open(t, "reply")
	require.NoError(t, f.generate(t, s.SessionID))
	assert.Equal(t, []string{"openai"}, f.consented(t))

	// 同じホスト（パス・大文字小文字・既定ポート違い）なら同意は維持される。
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://LLM.example:443/v2/")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, f.consented(t))

	// ホストが変わると同意は無効（保存からも外れる）。API キーは維持される。
	cfg, err := f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://other.example/v1")
	require.NoError(t, err)
	assert.Empty(t, cfg.ConsentedProviders)
	assert.True(t, cfg.HasAPIKey)
	requireCode(t, "AI_CONSENT_REQUIRED", f.generate(t, s.SessionID))
	v, ok, err := f.Store.GetSetting(t.Context(), store.SettingAIConsent)
	require.NoError(t, err)
	require.True(t, ok)
	assert.NotContains(t, v, "openai")

	// 元のホストに戻しても、改めて同意するまでは有効にならない。
	_, err = f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://llm.example/v1")
	require.NoError(t, err)
	assert.Empty(t, f.consented(t))
	requireCode(t, "AI_CONSENT_REQUIRED", f.generate(t, s.SessionID))

	cfg, err = f.ai.RecordAIConsent(t.Context(), "openai")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, cfg.ConsentedProviders)
	require.NoError(t, f.generate(t, s.SessionID))
}

func TestConsentIsPerDestinationHost(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", "https://a.example/v1")
	require.NoError(t, err)
	cfg, err := f.ai.RecordAIConsent(t.Context(), "openai")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, cfg.ConsentedProviders)

	_, err = f.ai.SetAIConfig(t.Context(), "openai", "m", "")
	require.NoError(t, err)
	assert.Empty(t, f.consented(t), "OpenAI 既定の送信先は a.example とは別")
}

func TestLegacyConsentIsTheDefaultDestination(t *testing.T) {
	f := newAIFixture(t)
	require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAIConsent, `{"provider":"openai","agreedAt":1730000000000}`))
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, f.consented(t), "旧形式は openai の既定 URL への同意")

	_, err = f.ai.SetAIConfig(t.Context(), "openai", "m", "https://llm.example/v1")
	require.NoError(t, err)
	assert.Empty(t, f.consented(t), "旧形式の同意はカスタム Base URL には効かない")
}

func TestConsentDroppedWhenSwitchingProviderClearsBaseURL(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "m", "https://llm.example/v1")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "openai")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic")
	require.NoError(t, err)

	cfg, err := f.ai.SetAIConfig(t.Context(), "anthropic", "claude", "https://ignored.example")
	require.NoError(t, err)
	assert.Equal(t, []string{"anthropic"}, cfg.ConsentedProviders, "anthropic の同意は維持。openai は送信先が変わるため無効")

	cfg, err = f.ai.SetAIConfig(t.Context(), "openai", "m", "https://llm.example/v1")
	require.NoError(t, err)
	assert.Equal(t, []string{"anthropic"}, cfg.ConsentedProviders, "openai は再同意が必要。anthropic の同意は残るが現在の設定では未使用")
}

func TestConsentTimestampKeptOnRerecord(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.RecordAIConsent(t.Context(), "openai")
	require.NoError(t, err)
	v1, _, err := f.Store.GetSetting(t.Context(), store.SettingAIConsent)
	require.NoError(t, err)
	f.Clock.Advance(3_600_000_000_000)
	_, err = f.ai.RecordAIConsent(t.Context(), "openai")
	require.NoError(t, err)
	v2, _, err := f.Store.GetSetting(t.Context(), store.SettingAIConsent)
	require.NoError(t, err)
	assert.Equal(t, v1, v2)
}
