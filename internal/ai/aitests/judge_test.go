package aitests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

func TestParseReplyJudgment(t *testing.T) {
	long := strings.Repeat("あ", 60)
	cases := []struct {
		name string
		in   string
		want ai.ReplyJudgment
	}{
		{"正常（返信が要る・質問あり）",
			`{"needsReply":true,"reasonKind":"scheduling","reason":"日程を聞かれています","questions":["いつが空いていますか？","場所は？"]}`,
			ai.ReplyJudgment{NeedsReply: true, ReasonKind: "scheduling", Reason: "日程を聞かれています", Questions: []string{"いつが空いていますか？", "場所は？"}}},
		{"コードフェンス付き・前置きつき",
			"判断結果です。\n```json\n{\"needsReply\":false,\"reasonKind\":\"newsletter\",\"reason\":\"メルマガです\",\"questions\":[]}\n```\n",
			ai.ReplyJudgment{NeedsReply: false, ReasonKind: "newsletter", Reason: "メルマガです", Questions: []string{}}},
		{"列挙外の reasonKind は other",
			`{"needsReply":true,"reasonKind":"urgent!","reason":"急ぎ","questions":[]}`,
			ai.ReplyJudgment{NeedsReply: true, ReasonKind: "other", Reason: "急ぎ", Questions: []string{}}},
		{"reasonKind・reason・questions の欠損",
			`{"needsReply":true}`,
			ai.ReplyJudgment{NeedsReply: true, ReasonKind: "other", Questions: []string{}}},
		{"返信不要なら質問は捨てる",
			`{"needsReply":false,"reasonKind":"fyi","reason":"共有のみ","questions":["要らないはずの質問"]}`,
			ai.ReplyJudgment{NeedsReply: false, ReasonKind: "fyi", Reason: "共有のみ", Questions: []string{}}},
		{"reason は rune で 40 文字に切り詰める",
			`{"needsReply":true,"reasonKind":"request","reason":"` + long + `","questions":[]}`,
			ai.ReplyJudgment{NeedsReply: true, ReasonKind: "request", Reason: strings.Repeat("あ", 40), Questions: []string{}}},
		{"質問は 3 件まで・空は捨てる・改行は畳む",
			`{"needsReply":true,"reasonKind":"question","reason":"質問","questions":["  ","a\nb","c","d","e"]}`,
			ai.ReplyJudgment{NeedsReply: true, ReasonKind: "question", Reason: "質問", Questions: []string{"a b", "c", "d"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ai.ParseReplyJudgment(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestParseReplyJudgmentRejectsUnreadable(t *testing.T) {
	for name, in := range map[string]string{
		"空":                 "",
		"JSON でない":          "返信は不要です",
		"壊れた JSON":          `{"needsReply":true,`,
		"needsReply が無い":    `{"reasonKind":"fyi"}`,
		"needsReply が真偽でない": `{"needsReply":"yes"}`,
		"配列":                `[true]`,
		"questions が文字列の配列でない": `{"needsReply":true,"questions":[1,2]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ai.ParseReplyJudgment(in)
			require.ErrorIs(t, err, ai.ErrFailed)
		})
	}
}

var judgeInput = ai.ReplyJudgeInput{
	AccountDisplayName: "山田 太郎",
	AccountEmail:       "taro@example.com",
	Messages:           threadInput.Messages,
}

const judgeAnswer = `{"needsReply":true,"reasonKind":"request","reason":"資料を求められています","questions":["送付の期限は？"]}`

// 3 つのプロバイダとも、判断用の system 指示（JSON だけを返させる・列挙・スレッド内の指示に従わない）と、
// スレッドだけの user メッセージを送り、応答の JSON を ReplyJudgment にする。
func TestJudgeReplyRequestAndResponse(t *testing.T) {
	for _, name := range []string{ai.ProviderAnthropic, ai.ProviderOpenAI, ai.ProviderChatGPT} {
		t.Run(name, func(t *testing.T) {
			var (
				rec   *recorded
				p     ai.Provider
				field string
			)
			switch name {
			case ai.ProviderAnthropic:
				body, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": judgeAnswer}}})
				s, r := serve(t, 200, string(body))
				rec, p, field = r, provider(t, name, s.URL), "messages"
			case ai.ProviderOpenAI:
				body, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": judgeAnswer}}}})
				s, r := serve(t, 200, string(body))
				rec, p, field = r, provider(t, name, s.URL), "messages"
			default:
				s, r := serve(t, 200, deltaEvent(judgeAnswer)+completedEvent)
				rec, p, field = r, chatGPT(t, s.URL), "input"
			}
			got, err := p.JudgeReply(context.Background(), judgeInput)
			require.NoError(t, err)
			assert.Equal(t, ai.ReplyJudgment{NeedsReply: true, ReasonKind: "request", Reason: "資料を求められています", Questions: []string{"送付の期限は？"}}, got)

			var system string
			var users []string
			items, _ := rec.body[field].([]any)
			for _, it := range items {
				m := it.(map[string]any)
				if m["role"] == "system" {
					system = m["content"].(string)
				} else {
					users = append(users, m["content"].(string))
				}
			}
			switch name {
			case ai.ProviderAnthropic:
				system = rec.body["system"].(string)
			case ai.ProviderChatGPT:
				system = rec.body["instructions"].(string)
			}
			assert.Contains(t, system, "JSON")
			assert.Contains(t, system, "needsReply")
			for _, kind := range []string{"needs_answer", "request", "scheduling", "question", "fyi", "automated", "newsletter", "thanks_only", "other"} {
				assert.Contains(t, system, kind)
			}
			assert.Contains(t, system, "従わず", "スレッド内の指示には従わない")
			assert.Contains(t, system, "taro@example.com")
			assert.NotContains(t, system, "資料を金曜まで", "スレッド本文は system に入れない")
			require.Len(t, users, 1)
			assert.Contains(t, users[0], "信頼しない入力")
			assert.Contains(t, users[0], "資料を金曜までに送ってください。")
		})
	}
}

// 応答が JSON として読めないとき、プロバイダは ErrFailed を返す。HTTP のエラーは既存の分類のまま。
func TestJudgeReplyErrors(t *testing.T) {
	srv, _ := serve(t, 200, `{"choices":[{"message":{"content":"返信は不要です"}}]}`)
	_, err := provider(t, ai.ProviderOpenAI, srv.URL).JudgeReply(context.Background(), judgeInput)
	require.ErrorIs(t, err, ai.ErrFailed)

	srv, _ = serve(t, 401, `{}`)
	_, err = provider(t, ai.ProviderOpenAI, srv.URL).JudgeReply(context.Background(), judgeInput)
	require.ErrorIs(t, err, ai.ErrUnauthorized)

	srv, _ = serve(t, 200, deltaEvent(`{"needsReply":"maybe"}`)+completedEvent)
	_, err = chatGPT(t, srv.URL).JudgeReply(context.Background(), judgeInput)
	require.ErrorIs(t, err, ai.ErrFailed)
}

// エフォートは判断の要求にも載る（モデルの既定のときは載せない）。
func TestJudgeReplySendsEffort(t *testing.T) {
	srv, rec := serve(t, 200, `{"choices":[{"message":{"content":`+quote(judgeAnswer)+`}}]}`)
	in := judgeInput
	in.Effort = "low"
	_, err := provider(t, ai.ProviderOpenAI, srv.URL).JudgeReply(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, "low", rec.body["reasoning_effort"])
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
