package aicoretests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
)

// TypeSafe AI（Jev。#314）は今のところ API キーだけを持ち、返信下書きの既定のプロバイダにはならない。
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
	// 同意は、Jev へ送るメールの範囲（#314 の要判断）が決まってから足す。それまでは取れない。
	_, err = f.ai.RecordAIConsent(ctx, "typesafe", "")
	requireCode(t, "VALIDATION", err)

	cfg, err = f.ai.ClearAIApiKey(ctx, "typesafe")
	require.NoError(t, err)
	assert.NotContains(t, cfg.APIKeyProviders, "typesafe")
	_, err = f.Secrets.Get(secrets.AIAPIKeyKey("typesafe"))
	require.ErrorIs(t, err, secrets.ErrNotFound)
}
