package agentcoretests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// 決められないことがあるときは、勝手に埋めず質問にし（下書きは作らない）、答えをもらってから下書きを作る。
func TestQuestionsThenAnswerCreatesDraft(t *testing.T) {
	f := newFixture(t)
	f.ai.judgeFn = func(int) (ai.ReplyJudgment, error) {
		return ai.ReplyJudgment{NeedsReply: true, ReasonKind: ai.ReasonScheduling, Reason: "日程を聞かれています", Questions: []string{"候補日は？", "場所は？"}}, nil
	}
	f.enable(t)

	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
	it := f.only(t)
	require.Equal(t, "asking", it.Status)
	assert.Equal(t, []string{"候補日は？", "場所は？"}, it.Questions)
	assert.Empty(t, it.DraftID)
	assert.Equal(t, 1, f.items(t).PendingCount)
	_, generates := f.ai.counts()
	assert.Zero(t, generates, "答えをもらうまで本文は作らない")
	assert.Zero(t, f.Gmail.CallCount("CreateDraft"))

	// 件数・長さの検証。
	_, err := f.svc.Agent.AnswerAgentQuestions(t.Context(), it.ID, []string{"来週火曜"})
	requireCode(t, "VALIDATION", err)
	_, err = f.svc.Agent.AnswerAgentQuestions(t.Context(), it.ID, []string{"来週火曜", "  "})
	requireCode(t, "VALIDATION", err)
	_, err = f.svc.Agent.AnswerAgentQuestions(t.Context(), it.ID, []string{"来週火曜", strings.Repeat("あ", 1001)})
	requireCode(t, "VALIDATION", err)
	assert.Equal(t, "asking", f.only(t).Status, "検証に落ちたら変わらない")
	_, generates = f.ai.counts()
	assert.Zero(t, generates)

	got, err := f.svc.Agent.AnswerAgentQuestions(t.Context(), it.ID, []string{"来週火曜", "本社の会議室"})
	require.NoError(t, err)
	assert.Equal(t, "ready", got.Status)
	assert.NotEmpty(t, got.DraftID)
	assert.Empty(t, got.Questions)
	assert.Equal(t, "日程を聞かれています", got.Reason)
	assert.Equal(t, 1, f.items(t).PendingCount)
	instr := f.ai.lastInstruction()
	assert.Contains(t, instr, "質問: 候補日は？")
	assert.Contains(t, instr, "回答: 来週火曜")
	assert.Contains(t, instr, "質問: 場所は？")
	assert.Contains(t, instr, "回答: 本社の会議室")
	d, err := f.Gmail.GetDraft(t.Context(), f.a.ID, got.DraftID)
	require.NoError(t, err)
	assert.Contains(t, d.Message.BodyHTML, "署名A")

	_, err = f.svc.Agent.AnswerAgentQuestions(t.Context(), it.ID, []string{"a", "b"})
	requireCode(t, "VALIDATION", err) // もう質問待ちではない
	f.requireNeverSent(t)
}

// 回答のあとの下書きづくりに失敗したら、エラーを返し、質問待ちのまま残す（もう一度答えられる）。
func TestAnswerFailureKeepsAsking(t *testing.T) {
	f := newFixture(t)
	f.ai.judgeFn = func(int) (ai.ReplyJudgment, error) {
		return ai.ReplyJudgment{NeedsReply: true, ReasonKind: ai.ReasonQuestion, Reason: "質問", Questions: []string{"金額は？"}}, nil
	}
	f.enable(t)
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
	it := f.only(t)

	f.ai.generateFn = func(int, ai.ReplyInput) (string, error) { return "", ai.ErrFailed }
	_, err := f.svc.Agent.AnswerAgentQuestions(t.Context(), it.ID, []string{"1000 円"})
	requireCode(t, "AI_FAILED", err)
	got := f.only(t)
	assert.Equal(t, "asking", got.Status)
	assert.Equal(t, []string{"金額は？"}, got.Questions)
	assert.Empty(t, got.DraftID)

	f.ai.generateFn = nil
	again, err := f.svc.Agent.AnswerAgentQuestions(t.Context(), it.ID, []string{"1000 円"})
	require.NoError(t, err)
	assert.Equal(t, "ready", again.Status)
}
