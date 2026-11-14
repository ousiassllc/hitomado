package aicoretests

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/store"
)

func storedConfig(t *testing.T, f *aiFixture) string {
	t.Helper()
	v, ok, err := f.Store.GetSetting(t.Context(), store.SettingAIFeatures)
	require.NoError(t, err)
	require.True(t, ok)
	return v
}

// 「モデルの既定」（省略・空・空白）はキーを書かず、設定にも出さない。選んだ段階は保存して返す（#111）。
func TestSetAIFeatureEffortRoundTrip(t *testing.T) {
	f := newAIFixture(t)

	cfg, err := apptest.SetDraftAI(f.ai, t.Context(), "anthropic", "claude-x", "", "")
	require.NoError(t, err)
	assert.Empty(t, cfg.Features.Draft.Effort)
	assert.NotContains(t, storedConfig(t, f), "effort")
	_, err = apptest.SetDraftAI(f.ai, t.Context(), "anthropic", "claude-x", "", "  ")
	require.NoError(t, err)
	assert.NotContains(t, storedConfig(t, f), "effort")

	cfg, err = apptest.SetDraftAI(f.ai, t.Context(), "anthropic", "claude-x", "", " xhigh ")
	require.NoError(t, err)
	assert.Equal(t, "xhigh", cfg.Features.Draft.Effort)
	assert.Contains(t, storedConfig(t, f), `"effort":"xhigh"`)
	got, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "xhigh", got.Features.Draft.Effort)

	// 渡し直さなければ「既定」に戻る（前の値を黙って引き継がない）。
	cfg, err = apptest.SetDraftAI(f.ai, t.Context(), "anthropic", "claude-y", "", "")
	require.NoError(t, err)
	assert.Empty(t, cfg.Features.Draft.Effort)
	assert.NotContains(t, storedConfig(t, f), "effort")
}

// プロバイダごとの集合の外は VALIDATION（保存しない）。
func TestSetAIFeatureRejectsOutOfRangeEffort(t *testing.T) {
	f := newAIFixture(t)
	for provider, bad := range map[string][]string{
		"anthropic": {"none", "minimal", "ultra", "HIGH"},
		"openai":    {"xhigh", "max", "none", "minimal"},
		"chatgpt":   {"xhigh", "max", "minimal"},
	} {
		for _, effort := range bad {
			_, err := apptest.SetDraftAI(f.ai, t.Context(), provider, "m", "", effort)
			ae := requireError(t, "VALIDATION", err)
			assert.NotContains(t, ae.Message, effort)
		}
	}
	_, ok, err := f.Store.GetSetting(t.Context(), store.SettingAIFeatures)
	require.NoError(t, err)
	assert.False(t, ok, "検証に失敗した設定は保存しない")

	for provider, good := range map[string][]string{
		"anthropic": {"low", "medium", "high", "xhigh", "max"},
		"openai":    {"low", "medium", "high"},
		"chatgpt":   {"low", "medium", "high"},
	} {
		for _, effort := range good {
			cfg, err := apptest.SetDraftAI(f.ai, t.Context(), provider, "m", "", effort)
			require.NoError(t, err, "%s %s", provider, effort)
			assert.Equal(t, effort, cfg.Features.Draft.Effort)
		}
	}
}

// 保存値が範囲外（手で書き換えられた・プロバイダを変えて使えなくなった）なら、読むときに無視して送らない。設定自体は残る。
func TestOutOfRangeStoredEffortIsIgnored(t *testing.T) {
	for _, tc := range []struct{ provider, stored string }{
		{"openai", "max"}, {"chatgpt", "xhigh"}, {"anthropic", "turbo"}, {"openai", "HIGH"},
	} {
		f := newAIFixture(t)
		f.ready(t)
		raw := fmt.Sprintf(`{"draft":{"provider":%q,"model":"gpt-test","effort":%q}}`, tc.provider, tc.stored)
		require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAIFeatures, raw))

		cfg, err := f.ai.GetAIConfig(t.Context())
		require.NoError(t, err)
		assert.Empty(t, cfg.Features.Draft.Effort, "%s %s", tc.provider, tc.stored)
		assert.Equal(t, "gpt-test", cfg.Features.Draft.Model, "設定は消さない")
	}

	f := newAIFixture(t)
	f.ready(t)
	require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAIFeatures,
		`{"draft":{"provider":"openai","model":"gpt-test","effort":"max"}}`))
	s := f.open(t, "reply")
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	require.NoError(t, err)
	require.Len(t, f.provider.inputs, 1)
	assert.Empty(t, f.provider.inputs[0].Effort, "範囲外の保存値はプロバイダへ送らない")
}

// 設定したエフォートは生成の入力に載る。未設定なら空（従来と同じ要求）。同意の単位は変わらない。
func TestGenerateReplyDraftPassesTheEffort(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	s := f.open(t, "reply")

	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	require.NoError(t, err)
	assert.Empty(t, f.provider.inputs[0].Effort)

	_, err = apptest.SetDraftAI(f.ai, t.Context(), "openai", "gpt-test", "https://llm.example/v1", "high")
	require.NoError(t, err)
	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r2", "")
	require.NoError(t, err, "エフォートを変えても同意は取り直さない")
	assert.Equal(t, "high", f.provider.inputs[1].Effort)
}

// エフォートを載せた要求をプロバイダが拒否したときは、「既定に戻す」案内つきの AI_FAILED。載せていないときは一般の失敗のまま。
func TestEffortRejectedGivesGuidance(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	s := f.open(t, "reply")
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) {
		return "", fmt.Errorf("%w: HTTP 400", ai.ErrRejected)
	}

	_, err := apptest.SetDraftAI(f.ai, t.Context(), "openai", "gpt-test", "https://llm.example/v1", "high")
	require.NoError(t, err)
	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	ae := requireError(t, "AI_FAILED", err)
	assert.Contains(t, ae.Message, "エフォート")
	assert.Contains(t, ae.Message, "既定")
	assert.Equal(t, apperr.ReasonEffortRejected, ae.Details[apperr.DetailReason])
	assert.NotContains(t, ae.Message, "400")

	_, err = apptest.SetDraftAI(f.ai, t.Context(), "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r2", "")
	ae = requireError(t, "AI_FAILED", err)
	assert.NotContains(t, ae.Message, "エフォート")
	assert.Nil(t, ae.Details)
}
