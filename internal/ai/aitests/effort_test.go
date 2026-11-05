package aitests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// generateWith は provider に effort つきの入力を渡し、偽サーバーが受け取った要求本文を返す。
func generateWith(t *testing.T, name, effort string) map[string]any {
	t.Helper()
	in := input
	in.Effort = effort
	var rec *recorded
	switch name {
	case ai.ProviderAnthropic:
		srv, r := serve(t, 200, `{"content":[{"type":"text","text":"ok"}]}`)
		rec = r
		_, err := provider(t, name, srv.URL).GenerateReply(context.Background(), in)
		require.NoError(t, err)
	case ai.ProviderOpenAI:
		srv, r := serve(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
		rec = r
		_, err := provider(t, name, srv.URL).GenerateReply(context.Background(), in)
		require.NoError(t, err)
	default:
		srv, r := serve(t, 200, deltaEvent("ok")+completedEvent)
		rec = r
		_, err := chatGPT(t, srv.URL).GenerateReply(context.Background(), in)
		require.NoError(t, err)
	}
	return rec.body
}

// エフォートは、プロバイダごとの項目で要求に載る（#111）。
func TestEffortIsSentInTheProviderSpecificField(t *testing.T) {
	anth := generateWith(t, ai.ProviderAnthropic, "xhigh")
	assert.Equal(t, map[string]any{"effort": "xhigh"}, anth["output_config"])
	assert.NotContains(t, anth, "reasoning_effort")

	oa := generateWith(t, ai.ProviderOpenAI, "medium")
	assert.Equal(t, "medium", oa["reasoning_effort"])
	assert.NotContains(t, oa, "output_config")

	gpt := generateWith(t, ai.ProviderChatGPT, "low")
	assert.Equal(t, map[string]any{"effort": "low"}, gpt["reasoning"])
	assert.NotContains(t, gpt, "reasoning_effort")
}

// 未設定（モデルの既定）のときは何も載せず、従来と同じ要求になる。
func TestNoEffortLeavesTheRequestUnchanged(t *testing.T) {
	for _, name := range allProviders {
		body := generateWith(t, name, "")
		for _, k := range []string{"output_config", "reasoning_effort", "reasoning"} {
			assert.NotContains(t, body, k, name)
		}
	}
	// ChatGPT の要求の項目の集合は変わらない（公式が未対応とする項目を足さない）。
	keys := make([]string, 0)
	for k := range generateWith(t, ai.ProviderChatGPT, "") {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"model", "instructions", "input", "store", "stream"}, keys)
}

func TestEffortScaleAndValidation(t *testing.T) {
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, ai.EffortScale(ai.ProviderAnthropic))
	for _, p := range []string{ai.ProviderOpenAI, ai.ProviderChatGPT} {
		assert.Equal(t, []string{"low", "medium", "high"}, ai.EffortScale(p))
	}
	assert.Nil(t, ai.EffortScale("gemini"))

	assert.True(t, ai.ValidEffort(ai.ProviderAnthropic, "max"))
	assert.False(t, ai.ValidEffort(ai.ProviderOpenAI, "max"), "none・minimal・xhigh・max は共通の段階に含めない")
	assert.False(t, ai.ValidEffort(ai.ProviderChatGPT, "xhigh"))
	assert.False(t, ai.ValidEffort(ai.ProviderOpenAI, ""), "空は段階ではなく「モデルの既定」")
	assert.False(t, ai.ValidEffort(ai.ProviderOpenAI, "HIGH"))
	assert.False(t, ai.ValidEffort("gemini", "high"))
}

// プロバイダが 400／422 で拒否したときは ErrRejected（ErrFailed でもある）。401・429・500 とは区別する。
func TestRejectedByTheProvider(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnprocessableEntity} {
		for _, name := range allProviders {
			srv, _ := serve(t, status, `{"error":{"message":"`+serverDetail+`"}}`)
			in := input
			in.Effort = "high"
			var err error
			if name == ai.ProviderChatGPT {
				_, err = chatGPT(t, srv.URL).GenerateReply(context.Background(), in)
			} else {
				_, err = provider(t, name, srv.URL).GenerateReply(context.Background(), in)
			}
			require.ErrorIs(t, err, ai.ErrRejected, "%s %d", name, status)
			require.ErrorIs(t, err, ai.ErrFailed)
			assertNoLeak(t, err)
		}
	}
	for status, want := range map[int]error{
		http.StatusUnauthorized:        ai.ErrUnauthorized,
		http.StatusTooManyRequests:     ai.ErrRateLimited,
		http.StatusInternalServerError: ai.ErrFailed,
	} {
		srv, _ := serve(t, status, `{}`)
		_, err := provider(t, ai.ProviderOpenAI, srv.URL).GenerateReply(context.Background(), input)
		require.ErrorIs(t, err, want)
		assert.NotErrorIs(t, err, ai.ErrRejected, "HTTP %d", status)
	}
}

// Anthropic の Models API の capabilities.effort から、モデルごとの段階（弱い順）だけを作る。
func TestListModelsAnthropicEfforts(t *testing.T) {
	srv, _ := serve(t, 200, `{"data":[
		{"id":"claude-new","display_name":"New","capabilities":{"effort":{"supported":true,
			"low":{"supported":true},"medium":{"supported":true},"high":{"supported":true},
			"xhigh":{"supported":true},"max":{"supported":true}}}},
		{"id":"claude-mid","capabilities":{"effort":{"supported":true,
			"low":{"supported":true},"medium":{"supported":false},"high":{"supported":true},
			"xhigh":null,"max":{"supported":true}}}},
		{"id":"claude-old","capabilities":{"effort":{"supported":false,
			"low":{"supported":true},"medium":{"supported":true},"high":{"supported":true},"max":{"supported":true}}}},
		{"id":"claude-none","capabilities":{"image_input":{"supported":true}}},
		{"id":"claude-bare"}
	],"has_more":false}`)
	got, err := listFor(ai.ProviderAnthropic, srv.URL)
	require.NoError(t, err)
	require.Len(t, got, 5)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, got[0].Efforts)
	assert.Equal(t, []string{"low", "high", "max"}, got[1].Efforts, "xhigh が null・medium が非対応のモデルは、対応する段階だけ")
	assert.Nil(t, got[2].Efforts, "supported が偽なら、段階があってもエフォートの設定は出さない")
	assert.Nil(t, got[3].Efforts)
	assert.Nil(t, got[4].Efforts)
}

// OpenAI 互換・ChatGPT の一覧はモデルごとの段階を返さないので、Efforts は常に空。
func TestListModelsOpenAIAndChatGPTHaveNoEfforts(t *testing.T) {
	srv, _ := serve(t, 200, `{"data":[{"id":"gpt-a","capabilities":{"effort":{"supported":true,"low":{"supported":true}}}}],
		"models":[{"slug":"gpt-5","visibility":"list","effort":{"supported":true}}]}`)
	for _, name := range []string{ai.ProviderOpenAI, ai.ProviderChatGPT} {
		got, err := listFor(name, srv.URL)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		for _, m := range got {
			assert.Nil(t, m.Efforts, name)
		}
	}
}
