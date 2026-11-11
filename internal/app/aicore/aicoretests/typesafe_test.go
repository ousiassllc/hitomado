package aicoretests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
)

// TypeSafe AI（Jev。#314）は API キーと同意を持つが、返信下書き・要約・質問のプロバイダにはならない（Inbox Zero の判定専用）。
func TestTypeSafeKeyButNotADraftProvider(t *testing.T) {
	f := newAIFixture(t)
	ctx := t.Context()

	cfg, err := f.ai.SetAIApiKey(ctx, "typesafe", "  "+testAPIKey+"  ")
	require.NoError(t, err)
	assert.Contains(t, cfg.APIKeyProviders, "typesafe")
	assert.NotContains(t, cfg.APIKeyProviders, "openai")
	got, err := f.Secrets.Get(secrets.AIAPIKeyKey("typesafe"))
	require.NoError(t, err)
	assert.Equal(t, testAPIKey, got, "前後の空白は除いて資格情報ストアへ")
	assert.Equal(t, "chatgpt", cfg.Provider, "キーを登録しても既定のプロバイダは変わらない")
	assert.False(t, cfg.HasAPIKey, "HasAPIKey は既定のプロバイダのもの")
	assert.NotContains(t, f.logs.String(), testAPIKey)

	_, err = f.ai.SetAIConfig(ctx, "typesafe", "jev", "", "")
	requireCode(t, "VALIDATION", err)
	_, err = f.ai.ListAIModels(ctx, "typesafe", "")
	requireCode(t, "VALIDATION", err)
	_, err = f.ai.SetAIApiKey(ctx, "typesafe", "  ")
	requireCode(t, "VALIDATION", err)
	// 同意は取れる（Base URL を持たない送信先）。
	cfg, err = f.ai.RecordAIConsent(ctx, "typesafe", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"typesafe"}, cfg.ConsentedProviders)

	cfg, err = f.ai.ClearAIApiKey(ctx, "typesafe")
	require.NoError(t, err)
	assert.NotContains(t, cfg.APIKeyProviders, "typesafe")
	_, err = f.Secrets.Get(secrets.AIAPIKeyKey("typesafe"))
	require.ErrorIs(t, err, secrets.ErrNotFound)
}

// typesafe の同意は既定のプロバイダに関わらず残り、ほかのプロバイダの設定の保存・同意でも消えない。Base URL は無視する。
func TestTypeSafeConsentSurvivesProviderChanges(t *testing.T) {
	f := newAIFixture(t)
	ctx := t.Context()

	cfg, err := f.ai.RecordAIConsent(ctx, "typesafe", "https://ignored.example")
	require.NoError(t, err)
	assert.Equal(t, []string{"typesafe"}, cfg.ConsentedProviders)

	for _, p := range []struct{ provider, baseURL string }{
		{"anthropic", ""}, {"openai", "https://llm.example/v1"}, {"openai", "https://other.example/v1"}, {"chatgpt", ""},
	} {
		cfg, err = f.ai.SetAIConfig(ctx, p.provider, "m", p.baseURL, "")
		require.NoError(t, err)
		assert.Contains(t, cfg.ConsentedProviders, "typesafe", p.provider)
	}
	cfg, err = f.ai.RecordAIConsent(ctx, "anthropic", "")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"typesafe", "anthropic"}, cfg.ConsentedProviders)

	// 保存値としても Base URL は持たない（同意の再記録は 1 件のまま）。
	cfg, err = f.ai.RecordAIConsent(ctx, "typesafe", "")
	require.NoError(t, err)
	assert.Len(t, cfg.ConsentedProviders, 2)
}
