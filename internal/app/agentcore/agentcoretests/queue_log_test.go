package agentcoretests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
)

// 待ち行列は容量 50。溢れた分は捨て、件数だけログに残す。
func TestQueueOverflowDropsAndLogsCountOnly(t *testing.T) {
	f := newFixture(t)
	f.ready(t)
	_, err := f.svc.Agent.SetAgentEnabled(t.Context(), true, true)
	require.NoError(t, err)

	var cands []syncer.NewMailCandidate
	for i := range 55 {
		id := "q" + string(rune('A'+i/26)) + string(rune('a'+i%26))
		f.putInbound(t, f.a, id, "m-"+id)
		cands = append(cands, syncer.NewMailCandidate{ThreadID: id, MessageID: "m-" + id})
	}
	f.svc.Agent.OnNewMail(t.Context(), f.a.ID, cands, 0) // ワーカーを起こす前に積む（最初の 50 件だけが入る）
	f.start(t)
	f.waitItems(t, 50)
	f.stop()

	assert.Len(t, f.items(t).Items, 50)
	judges, _ := f.ai.counts()
	assert.Equal(t, 50, judges)
	assert.Contains(t, f.logs.String(), "queue full")
	assert.Contains(t, f.logs.String(), "count=5")
}

// どの経路でも、ログに件名・本文・差出人・質問・回答・理由・下書きの文面を出さない。
func TestLogsNeverContainMailContent(t *testing.T) {
	f := newFixture(t)
	f.ai.judgeFn = func(call int) (ai.ReplyJudgment, error) {
		switch call {
		case 1: // 質問
			return ai.ReplyJudgment{NeedsReply: true, ReasonKind: ai.ReasonQuestion, Reason: secretReason, Questions: []string{secretQuest}}, nil
		case 2: // 返信不要
			return ai.ReplyJudgment{NeedsReply: false, ReasonKind: ai.ReasonFYI, Reason: secretReason, Questions: []string{}}, nil
		case 3: // 失敗（プロバイダのエラーに本文が混じっても出さない）
			return ai.ReplyJudgment{}, fmt.Errorf("%w: %s%s", ai.ErrFailed, secretBody, secretSubject)
		}
		return ai.ReplyJudgment{NeedsReply: true, ReasonKind: ai.ReasonRequest, Reason: secretReason, Questions: []string{}}, nil
	}
	f.enable(t)
	for i, id := range []string{"t1", "t2", "t3", "t4"} {
		if id != "t1" {
			f.putInbound(t, f.a, id, "m"+id)
		}
		f.newMail(f.a.ID, id, map[bool]string{true: "m1", false: "m" + id}[i == 0])
		f.waitItems(t, i+1)
	}
	var asking, skipped, failed string
	for _, it := range f.items(t).Items {
		switch it.Status {
		case "asking":
			asking = it.ID
		case "skipped":
			skipped = it.ID
		case "failed":
			failed = it.ID
		}
	}
	_, err := f.svc.Agent.AnswerAgentQuestions(t.Context(), asking, []string{secretAnswer})
	require.NoError(t, err)
	_, err = f.svc.Agent.ReconsiderAgentItem(t.Context(), skipped)
	require.NoError(t, err)
	f.ai.generateFn = func(int, ai.ReplyInput) (string, error) { return "", fmt.Errorf("%w: %s", ai.ErrFailed, secretDraft) }
	_, err = f.svc.Agent.ReconsiderAgentItem(t.Context(), failed)
	require.Error(t, err)
	f.stop()

	out := f.logs.String()
	require.NotEmpty(t, out, "ログは出ている（判断・下書き・失敗）")
	assert.Empty(t, containsAny(out, secretSubject, secretFrom, "alice@x.example", secretBody, secretReason, secretAnswer, secretQuest, secretDraft),
		"ログに本文・件名・差出人などが含まれる:\n%s", out)
	assert.Contains(t, out, "reasonKind=")
	assert.Contains(t, out, "judged")
	f.requireNeverSent(t)
}
