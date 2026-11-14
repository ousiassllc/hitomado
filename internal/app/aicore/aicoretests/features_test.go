package aicoretests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// 「AI で下書き」は、機能ごとに選んだ AI・モデル・エフォートで生成する（返信・要約・質問・新規作成で共通）。
func TestDraftUsesTheSelectedAIAndModel(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t) // openai + Base URL
	_, err := f.ai.SetAIApiKey(t.Context(), "anthropic", "sk-anthropic")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)
	_, err = f.ai.SetAIFeature(t.Context(), dto.AIFeatureDraft, "anthropic", "claude-x", "high")
	require.NoError(t, err)

	s := f.open(t, "reply")
	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	require.NoError(t, err)
	assert.Equal(t, [4]string{"anthropic", "claude-x", "", "sk-anthropic"}, [4]string{f.factory.provider, f.factory.model, f.factory.baseURL, f.factory.key})
	assert.Equal(t, "high", f.provider.inputs[0].Effort)

	_, err = f.ai.SummarizeThread(t.Context(), f.ref(), "r2")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", f.factory.provider, "要約も「AI で下書き」と同じ設定を使う")
}

// Inbox Zero に選んだ AI は、「AI で下書き」の選択とは別に読める（#377 が判定に使う入口）。
func TestInboxZeroSelectionIsReadable(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	_, err := f.ai.SetAIFeature(t.Context(), dto.AIFeatureInboxZero, "openai", "gpt-zero", "")
	require.NoError(t, err)
	sel, saved, err := f.ai.FeatureAI(t.Context(), dto.AIFeatureInboxZero)
	require.NoError(t, err)
	assert.True(t, saved)
	assert.Equal(t, aicore.SelectedAI{Provider: "openai", Model: "gpt-zero", BaseURL: "https://llm.example/v1"}, sel)
	_, saved, err = f.ai.FeatureAI(t.Context(), dto.AIFeatureDraft)
	require.NoError(t, err)
	assert.True(t, saved, "ready が「AI で下書き」も openai に選んでいる")
}

// つないでいない（API キーを外した・ログアウトした）AI を選んでいても、ほかのつないである AI へ切り替えず、AI_NOT_CONFIGURED で止まる。
func TestDraftNeverSwitchesToAnotherAIWhenTheSelectedOneIsDisconnected(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)                                                           // 「AI で下書き」は openai
	_, err := f.ai.SetAIApiKey(t.Context(), "anthropic", "sk-anthropic") // つないである別の AI
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)
	_, err = f.ai.ClearAIApiKey(t.Context(), "openai") // 選んでいる AI の接続を外す
	require.NoError(t, err)

	s := f.open(t, "reply")
	err = f.generate(t, s.SessionID)
	requireCode(t, "AI_NOT_CONFIGURED", err)
	_, err = f.ai.SummarizeThread(t.Context(), f.ref(), "r2")
	requireCode(t, "AI_NOT_CONFIGURED", err)
	assert.Empty(t, f.provider.inputs, "別の AI へ送らない")
	assert.Empty(t, f.factory.provider, "プロバイダを作ってもいない")
	cfg, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, dto.AIFeatureSetting{Provider: "openai", Model: "gpt-test"}, cfg.Features.Draft, "選択は残る")
}

// ChatGPT のログアウトでも同じ（再ログインするまで、選んだ ChatGPT のまま止まる）。
func TestDraftStopsWhenTheSelectedChatGPTIsSignedOut(t *testing.T) {
	f := newAIFixture(t)
	f.chatgptReady(t)
	cfg, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "anthropic", "sk-anthropic")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)
	_, err = f.ai.SignOutChatGPT(t.Context(), cfg.ChatGPTAccounts[0].ID)
	require.NoError(t, err)

	s := f.open(t, "reply")
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) { return "should not run", nil }
	requireCode(t, "AI_NOT_CONFIGURED", f.generate(t, s.SessionID))
	assert.Empty(t, f.factory.provider)
}

// 機能を選んでいない（未保存）ときは、つないである AI があっても生成しない。
func TestDraftRequiresAnExplicitSelection(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.SetAIApiKey(t.Context(), "anthropic", "sk-anthropic")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)

	s := f.open(t, "reply")
	requireCode(t, "AI_NOT_CONFIGURED", f.generate(t, s.SessionID))
	assert.Empty(t, f.factory.provider)
}
