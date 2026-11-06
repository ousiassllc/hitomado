package aicoretests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

func turns(roles ...string) []dto.ThreadAITurn {
	out := make([]dto.ThreadAITurn, len(roles))
	for i, r := range roles {
		out[i] = dto.ThreadAITurn{Role: r, Text: "t" + string(rune('a'+i))}
	}
	return out
}

// 質問は前後の空白を除き、history・スレッドの内容とともにプロバイダへ渡る（F-18）。nil・空の history も通る。
func TestAskAboutThreadPassesQuestionAndHistory(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)

	res, err := f.ai.AskAboutThread(t.Context(), f.ref(), "r1", "  期限はいつ？\n", nil)
	require.NoError(t, err)
	assert.Equal(t, dto.ThreadAIResult{Text: "- 回答", IncludedCount: 1, TotalCount: 1}, res)
	require.Len(t, f.provider.questions, 1)
	q := f.provider.questions[0]
	assert.Equal(t, "期限はいつ？", q.Question)
	assert.Empty(t, q.History)
	require.Len(t, q.Messages, 1)
	assert.Empty(t, f.provider.summaries)

	_, err = f.ai.AskAboutThread(t.Context(), f.ref(), "r2", "続きは？", turns("user", "assistant", "user", "assistant"))
	require.NoError(t, err)
	q = f.provider.questions[1]
	assert.Equal(t, []ai.ThreadTurn{
		{Role: "user", Text: "ta"}, {Role: "assistant", Text: "tb"}, {Role: "user", Text: "tc"}, {Role: "assistant", Text: "td"},
	}, q.History)
}

// 質問・history の検証は通信前に VALIDATION。エラー文面に内容を含めない。
func TestAskAboutThreadValidatesBeforeSending(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)

	long := dto.ThreadAITurn{Role: "user", Text: strings.Repeat("秘", ai.MaxHistoryRunes)}
	ans := dto.ThreadAITurn{Role: "assistant", Text: "あ"}
	tooMany := make([]dto.ThreadAITurn, 0, ai.MaxHistoryTurns+2)
	for range ai.MaxHistoryTurns/2 + 1 {
		tooMany = append(tooMany, dto.ThreadAITurn{Role: "user", Text: "q"}, ans)
	}
	cases := map[string]struct {
		question string
		history  []dto.ThreadAITurn
	}{
		"空の質問":          {"", nil},
		"空白だけの質問":       {" \n\t", nil},
		"2,000 文字超の質問":  {strings.Repeat("秘", ai.MaxQuestionRunes+1), nil},
		"先頭が assistant": {"q", turns("assistant", "user")},
		"交互でない":         {"q", turns("user", "user")},
		"最後が user":      {"q", turns("user", "assistant", "user")},
		"未知の role":      {"q", turns("user", "system")},
		"空の発言":          {"q", []dto.ThreadAITurn{{Role: "user", Text: " "}, ans}},
		"21 件以上":        {"q", tooMany},
		"合計 20,000 文字超": {"q", []dto.ThreadAITurn{long, {Role: "assistant", Text: "あ"}}},
	}
	for name, tc := range cases {
		_, err := f.ai.AskAboutThread(t.Context(), f.ref(), "r-"+name, tc.question, tc.history)
		ae := requireError(t, "VALIDATION", err)
		assert.NotContains(t, ae.Message, "秘", name)
	}
	assert.Empty(t, f.provider.questions, "検証に失敗したときはプロバイダへ送らない")

	// 境界: 質問 2,000 文字・20 件（10 往復）・合計 20,000 文字ちょうどは通る。
	max := make([]dto.ThreadAITurn, 0, ai.MaxHistoryTurns)
	for range ai.MaxHistoryTurns / 2 {
		max = append(max, dto.ThreadAITurn{Role: "user", Text: strings.Repeat("あ", 1000)}, ans)
	}
	max[0].Text += strings.Repeat("あ", ai.MaxHistoryRunes-10*1000-10) // 合計をちょうど 20,000 にする
	_, err := f.ai.AskAboutThread(t.Context(), f.ref(), "r-ok", strings.Repeat("あ", ai.MaxQuestionRunes), max)
	require.NoError(t, err)
}

// 前提（未設定・未同意）と requestId は要約と同じ。
func TestAskAboutThreadPreconditions(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.AskAboutThread(t.Context(), f.ref(), "r1", "q", nil)
	requireCode(t, "AI_NOT_CONFIGURED", err)
	f.ready(t)
	_, err = f.ai.AskAboutThread(t.Context(), f.ref(), "", "q", nil)
	requireCode(t, "VALIDATION", err)
}
