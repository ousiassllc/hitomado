package aitests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

var chatInput = ai.ClassifyInput{
	State:        "From: Shop <shop@example.com>\nSubject: セール\n\n本文",
	Instructions: "このメールの種類として、最も当てはまるものを 1 つ選んでください。",
	Criteria:     []ai.Criterion{{Name: "ad", Description: "広告のメール"}, {Name: "notification", Description: "通知だけのメール"}, {Name: "other", Description: "通常のメール"}},
}

// openAIChat は Chat Completions の応答の形（assistant の本文だけ）。
func openAIChat(content string) string {
	b, _ := json.Marshal(content)
	return `{"choices":[{"message":{"role":"assistant","content":` + string(b) + `}}]}`
}

func chatClassifier(t *testing.T, provider, base string) ai.Classifier {
	t.Helper()
	c, err := ai.NewChatClassifierWithClient(provider, "model-x", base, apiKey, nil)
	require.NoError(t, err)
	return c
}

// OpenAI 互換: 要求は Chat Completions。system に選択肢（渡した順）と出力形式、メールの内容は user の <email> の中だけ。
// 確信度を選んだ選択肢の確率として返す。
func TestChatClassifierOpenAI(t *testing.T) {
	srv, rec := serve(t, 200, openAIChat(`{"choice":"ad","confidence":0.93}`))
	ans, err := chatClassifier(t, ai.ProviderOpenAI, srv.URL).Classify(t.Context(), chatInput)
	require.NoError(t, err)

	assert.Equal(t, "/chat/completions", rec.path)
	assert.Equal(t, "Bearer "+apiKey, rec.header.Get("Authorization"))
	assert.Equal(t, "model-x", rec.body["model"])
	msgs := rec.body["messages"].([]any)
	require.Len(t, msgs, 2)
	system, user := msgs[0].(map[string]any), msgs[1].(map[string]any)
	assert.Equal(t, "system", system["role"])
	sys := system["content"].(string)
	assert.Contains(t, sys, chatInput.Instructions)
	assert.Less(t, strings.Index(sys, "- ad: 広告のメール"), strings.Index(sys, "- notification: 通知だけのメール"))
	assert.Less(t, strings.Index(sys, "- notification:"), strings.Index(sys, "- other: 通常のメール"))
	assert.Contains(t, sys, `"confidence"`)
	assert.NotContains(t, sys, "shop@example.com", "メールの内容は system 指示に入れない")
	assert.Equal(t, "user", user["role"])
	assert.Equal(t, "<email>\n"+chatInput.State+"\n</email>", user["content"])
	assert.NotContains(t, rec.body, "reasoning_effort", "分類にはエフォートを載せない")

	assert.Equal(t, ai.ClassifyAnswer{Choice: "ad", Probabilities: map[string]float64{"ad": 0.93}}, ans)
}

// Anthropic: Messages API。ChatGPT プラン: Responses API（SSE）。どちらも同じ答えの形に読む。
func TestChatClassifierAnthropicAndChatGPT(t *testing.T) {
	srv, rec := serve(t, 200, `{"content":[{"type":"text","text":"{\"choice\":\"notification\",\"confidence\":0.85}"}]}`)
	ans, err := chatClassifier(t, ai.ProviderAnthropic, srv.URL).Classify(t.Context(), chatInput)
	require.NoError(t, err)
	assert.Equal(t, "/v1/messages", rec.path)
	assert.Equal(t, apiKey, rec.header.Get("x-api-key"))
	assert.Contains(t, rec.body["system"], "- notification: 通知だけのメール")
	assert.Equal(t, "notification", ans.Choice)
	assert.InDelta(t, 0.85, ans.Probabilities["notification"], 1e-9)

	stream := createdEvent + deltaEvent(`{"choice":"other",`) + deltaEvent(`"confidence":1}`) + completedEvent
	srv, rec = serve(t, 200, stream)
	ans, err = chatClassifier(t, ai.ProviderChatGPT, srv.URL).Classify(t.Context(), chatInput)
	require.NoError(t, err)
	assert.Equal(t, "/responses", rec.path)
	assert.Equal(t, "Bearer "+apiKey, rec.header.Get("Authorization"))
	assert.Equal(t, ai.ClassifyAnswer{Choice: "other", Probabilities: map[string]float64{"other": 1}}, ans)
}

// 前後の文章・コードブロックは許し、名前の大文字小文字は選択肢に合わせる。選択肢にない名前・確信度が無い／範囲外・JSON でない応答は ErrFailed。
func TestChatClassifierParsesAnswers(t *testing.T) {
	for content, want := range map[string]string{
		"```json\n{\"choice\": \"AD\", \"confidence\": 0.9}\n```": "ad",
		"判定は次のとおりです。{\"choice\":\"other\",\"confidence\":0.2} 以上": "other",
	} {
		srv, _ := serve(t, 200, openAIChat(content))
		ans, err := chatClassifier(t, ai.ProviderOpenAI, srv.URL).Classify(t.Context(), chatInput)
		require.NoError(t, err, content)
		assert.Equal(t, want, ans.Choice, content)
	}
	for _, content := range []string{
		`{"choice":"spam","confidence":0.9}`, `{"choice":"ad"}`, `{"choice":"ad","confidence":1.5}`, `{"choice":"ad","confidence":-0.1}`,
		`{"choice":"ad","confidence":"high"}`, `広告です`, `{"choice":`, ``,
	} {
		srv, _ := serve(t, 200, openAIChat(content))
		_, err := chatClassifier(t, ai.ProviderOpenAI, srv.URL).Classify(t.Context(), chatInput)
		require.ErrorIs(t, err, ai.ErrFailed, content)
		assert.NotContains(t, err.Error(), "広告です", "応答の本文はエラーに含めない")
	}
}

// HTTP のエラーは Provider と同じ分類。キーも応答本文もエラーに含めない。
func TestChatClassifierErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{{401, ai.ErrUnauthorized}, {403, ai.ErrUnauthorized}, {429, ai.ErrRateLimited}, {500, ai.ErrFailed}} {
		srv, _ := serve(t, tc.status, `{"error":"secret-body-detail"}`)
		_, err := chatClassifier(t, ai.ProviderOpenAI, srv.URL).Classify(t.Context(), chatInput)
		require.ErrorIs(t, err, tc.want, tc.status)
		assert.NotContains(t, fmt.Sprint(err), apiKey)
		assert.NotContains(t, fmt.Sprint(err), "secret-body-detail")
	}
}

// 工場: typesafe は Jev（モデル・Base URL を使わない）、チャット系はモデルが必須で、未対応のプロバイダ・不正な Base URL はエラー。
func TestNewClassifierFor(t *testing.T) {
	c, err := ai.NewClassifierFor(ai.ProviderTypeSafe, "", "", apiKey)
	require.NoError(t, err)
	assert.NotNil(t, c)
	for _, p := range []string{ai.ProviderAnthropic, ai.ProviderOpenAI, ai.ProviderChatGPT} {
		c, err = ai.NewClassifierFor(p, "m", "", apiKey)
		require.NoError(t, err, p)
		assert.NotNil(t, c, p)
		_, err = ai.NewClassifierFor(p, "", "", apiKey)
		require.Error(t, err, p+": モデルが必須")
	}
	_, err = ai.NewClassifierFor("gemini", "m", "", apiKey)
	require.Error(t, err)
	_, err = ai.NewClassifierFor(ai.ProviderOpenAI, "m", "http://evil.example/v1", apiKey)
	require.ErrorIs(t, err, ai.ErrInsecureBaseURL)
}
