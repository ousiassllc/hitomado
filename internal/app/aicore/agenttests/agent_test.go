// Package agenttests は AI の入口のうち、エージェントモード（#372）が使うもの（Ready・AgreeCurrent・JudgeThread・DraftReplyText）のテスト。
// 設定・同意・生成そのものの単体のテストは internal/app/aicore/aicoretests にある。
package agenttests

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// Ready は、設定（キー・モデル）と現在の送信先への同意が揃ったときだけ真。未設定・未同意はエラーにせず偽。
func TestReadyRequiresConfigAndConsent(t *testing.T) {
	f := newAIFixture(t)
	ready, err := f.ai.Ready(t.Context())
	require.NoError(t, err)
	assert.False(t, ready, "未設定")

	_, err = f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	ready, err = f.ai.Ready(t.Context())
	require.NoError(t, err)
	assert.False(t, ready, "同意なし")

	_, err = f.ai.RecordAIConsent(t.Context(), "openai", "https://llm.example/v1")
	require.NoError(t, err)
	ready, err = f.ai.Ready(t.Context())
	require.NoError(t, err)
	assert.True(t, ready)

	// 送信先（Base URL のホスト）を変えると、同意は無効になる。
	_, err = f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://other.example/v1", "")
	require.NoError(t, err)
	ready, err = f.ai.Ready(t.Context())
	require.NoError(t, err)
	assert.False(t, ready)
}

// AgreeCurrent は、設定済みなら現在のプロバイダ・送信先への同意を記録し、未設定は AI_NOT_CONFIGURED。
func TestAgreeCurrentRecordsConsentForTheConfiguredDestination(t *testing.T) {
	f := newAIFixture(t)
	requireCode(t, "AI_NOT_CONFIGURED", f.ai.AgreeCurrent(t.Context()))

	_, err := f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	require.NoError(t, f.ai.AgreeCurrent(t.Context()))
	ready, err := f.ai.Ready(t.Context())
	require.NoError(t, err)
	assert.True(t, ready)

	// 別の送信先へ設定を変えたあとは、その送信先に対する同意を記録する。
	_, err = f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://other.example/v1", "")
	require.NoError(t, err)
	require.NoError(t, f.ai.AgreeCurrent(t.Context()))
	ready, err = f.ai.Ready(t.Context())
	require.NoError(t, err)
	assert.True(t, ready)
	cfg, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"openai"}, cfg.ConsentedProviders)
}

// JudgeThread・DraftReplyText は、前提（設定・同意）が無ければ通信せずエラーにし、あればスレッドと設定を載せてプロバイダを呼ぶ。
func TestJudgeThreadAndDraftReplyTextPreconditionsAndInput(t *testing.T) {
	f := newAIFixture(t)
	ref := dto.ThreadRef{AccountID: f.a.ID, ThreadID: threadID}

	_, err := f.ai.JudgeThread(t.Context(), ref)
	requireCode(t, "AI_NOT_CONFIGURED", err)
	_, err = f.ai.DraftReplyText(t.Context(), ref, "")
	requireCode(t, "AI_NOT_CONFIGURED", err)

	_, err = f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.JudgeThread(t.Context(), ref)
	requireCode(t, "AI_CONSENT_REQUIRED", err)
	_, err = f.ai.DraftReplyText(t.Context(), ref, "")
	requireCode(t, "AI_CONSENT_REQUIRED", err)
	assert.Empty(t, f.provider.judgeInputs, "前提を満たすまで通信しない")
	assert.Empty(t, f.provider.inputs)

	f.ready(t)
	j, err := f.ai.JudgeThread(t.Context(), ref)
	require.NoError(t, err)
	assert.True(t, j.NeedsReply)
	require.Len(t, f.provider.judgeInputs, 1)
	in := f.provider.judgeInputs[0]
	assert.Equal(t, f.a.Email, in.AccountEmail)
	require.Len(t, in.Messages, 1)
	assert.Equal(t, "Alice", in.Messages[0].From)
	assert.Equal(t, [4]string{"openai", "gpt-test", "https://llm.example/v1", testAPIKey},
		[4]string{f.factory.provider, f.factory.model, f.factory.baseURL, f.factory.key})

	text, err := f.ai.DraftReplyText(t.Context(), ref, "  利用者の回答  ")
	require.NoError(t, err)
	assert.Equal(t, "ok", text)
	require.Len(t, f.provider.inputs, 1)
	assert.Equal(t, "利用者の回答", f.provider.inputs[0].Instruction)
	assert.Contains(t, f.provider.inputs[0].SignatureText, "Sig A", "署名は重複して書かせないためだけに渡す")

	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) { return "  ", nil }
	_, err = f.ai.DraftReplyText(t.Context(), ref, "")
	requireCode(t, "AI_FAILED", err)

	_, err = f.ai.JudgeThread(t.Context(), dto.ThreadRef{AccountID: f.a.ID, ThreadID: "none"})
	requireCode(t, "NOT_FOUND", err)
}

// ChatGPT で 401 を受けたら、トークンを更新して 1 度だけやり直す（GenerateReplyDraft と同じ経路）。
func TestJudgeThreadRefreshesChatGPTTokenOnce(t *testing.T) {
	f := newAIFixture(t)
	f.chatgptReady(t)
	ref := dto.ThreadRef{AccountID: f.a.ID, ThreadID: threadID}
	var keys []string
	f.Deps.AI = func(_, _, _, key string) (ai.Provider, error) {
		keys = append(keys, key)
		return f.provider, nil
	}
	calls := 0
	f.provider.judgeFn = func(context.Context, ai.ReplyJudgeInput) (ai.ReplyJudgment, error) {
		calls++
		if calls == 1 {
			return ai.ReplyJudgment{}, fmt.Errorf("%w: HTTP 401", ai.ErrUnauthorized)
		}
		return ai.ReplyJudgment{NeedsReply: false, ReasonKind: ai.ReasonFYI, Questions: []string{}}, nil
	}
	j, err := f.ai.JudgeThread(t.Context(), ref)
	require.NoError(t, err)
	assert.False(t, j.NeedsReply)
	assert.Equal(t, []string{chatgptToken, chatgptRefresh}, keys)
	assert.Equal(t, 1, f.ChatGPT.Refreshes)
}

// 判断の失敗はエラー写像（AI_FAILED など）で返る。ctx の中止は CANCELLED。
func TestJudgeThreadMapsErrors(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	ref := dto.ThreadRef{AccountID: f.a.ID, ThreadID: threadID}
	f.provider.judgeFn = func(context.Context, ai.ReplyJudgeInput) (ai.ReplyJudgment, error) {
		return ai.ReplyJudgment{}, fmt.Errorf("%w: 判断の応答を読めません", ai.ErrFailed)
	}
	_, err := f.ai.JudgeThread(t.Context(), ref)
	requireCode(t, "AI_FAILED", err)

	f.provider.judgeFn = func(context.Context, ai.ReplyJudgeInput) (ai.ReplyJudgment, error) {
		return ai.ReplyJudgment{}, ai.ErrRateLimited
	}
	_, err = f.ai.JudgeThread(t.Context(), ref)
	requireCode(t, "RATE_LIMITED", err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = f.ai.JudgeThread(ctx, ref)
	requireCode(t, "CANCELLED", err)
}
