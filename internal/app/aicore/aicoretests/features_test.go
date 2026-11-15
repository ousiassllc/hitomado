package aicoretests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// 「AI で下書き」は、機能ごとに選んだ AI・モデル・エフォートで生成する（返信・新規作成で共通）。要約・質問は別の選択（threadAi）を使う。
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
	assert.Equal(t, "openai", f.factory.provider, "要約は「AI で下書き」ではなく、「要約・質問」に選んだ AI を使う")
}

// 要約・質問は、「要約・質問」（threadAi）に選んだ AI・モデル・エフォートで実行する。「AI で下書き」の選択は使わない。
func TestThreadAIUsesItsOwnSelection(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t) // draft・threadAi とも openai
	_, err := f.ai.SetAIApiKey(t.Context(), "anthropic", "sk-anthropic")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)
	_, err = f.ai.SetAIFeature(t.Context(), dto.AIFeatureThreadAI, "anthropic", "claude-sum", "max")
	require.NoError(t, err)

	_, err = f.ai.SummarizeThread(t.Context(), f.ref(), "r1")
	require.NoError(t, err)
	assert.Equal(t, [4]string{"anthropic", "claude-sum", "", "sk-anthropic"}, [4]string{f.factory.provider, f.factory.model, f.factory.baseURL, f.factory.key})
	require.Len(t, f.provider.summaries, 1)
	assert.Equal(t, "max", f.provider.summaries[0].Effort)

	_, err = f.ai.AskAboutThread(t.Context(), f.ref(), "r2", "いつまで？", nil)
	require.NoError(t, err)
	assert.Equal(t, "claude-sum", f.factory.model, "質問も同じ選択を使う")

	s := f.open(t, "reply")
	require.NoError(t, f.generate(t, s.SessionID))
	assert.Equal(t, [2]string{"openai", "gpt-test"}, [2]string{f.factory.provider, f.factory.model}, "「AI で下書き」は draft の選択のまま")
}

// 「AI で下書き」だけ選んでいる（要約・質問は未選択）ときは、draft へ切り替えず AI_NOT_CONFIGURED で止まる。
func TestThreadAIDoesNotFallBackToTheDraftSelection(t *testing.T) {
	g := newAIFixture(t)
	_, err := apptest.SetDraftAI(g.ai, t.Context(), "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	_, err = g.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	_, err = g.ai.RecordAIConsent(t.Context(), "openai", "https://llm.example/v1")
	require.NoError(t, err)

	_, err = g.ai.SummarizeThread(t.Context(), g.ref(), "r1")
	ae := requireError(t, "AI_NOT_CONFIGURED", err)
	assert.Contains(t, ae.Message, "「要約・質問」")
	assert.Contains(t, ae.Message, "機能ごとに使う AI")
	_, err = g.ai.AskAboutThread(t.Context(), g.ref(), "r2", "いつまで？", nil)
	requireCode(t, "AI_NOT_CONFIGURED", err)
	assert.Empty(t, g.provider.summaries, "draft の AI へは送らない")
	assert.Empty(t, g.factory.provider, "プロバイダを作ってもいない")

	// 「AI で下書き」は同じ状態で動く。
	s := g.open(t, "reply")
	require.NoError(t, g.generate(t, s.SessionID))
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
