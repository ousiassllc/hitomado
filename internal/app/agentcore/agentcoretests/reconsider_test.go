package agentcoretests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// 返信不要と判断したものは、「やっぱり返信する」で返信が要る前提の下書き作りにやり直せる。
func TestReconsiderSkippedCreatesDraft(t *testing.T) {
	f := newFixture(t)
	f.ai.judgeFn = func(int) (ai.ReplyJudgment, error) {
		return ai.ReplyJudgment{NeedsReply: false, ReasonKind: ai.ReasonFYI, Reason: "共有のみ", Questions: []string{}}, nil
	}
	f.enable(t)
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
	it := f.only(t)
	require.Equal(t, "skipped", it.Status)

	got, err := f.svc.Agent.ReconsiderAgentItem(t.Context(), it.ID)
	require.NoError(t, err)
	assert.Equal(t, "ready", got.Status, "AI がまだ不要と言っても、利用者が返信すると決めたので下書きを作る")
	assert.NotEmpty(t, got.DraftID)
	assert.Equal(t, 1, f.items(t).PendingCount)
	assert.Equal(t, 1, f.Gmail.CallCount("CreateDraft"))

	_, err = f.svc.Agent.ReconsiderAgentItem(t.Context(), it.ID)
	requireCode(t, "VALIDATION", err) // ready のものはやり直せない
	f.requireNeverSent(t)
}

// やり直しで決められないことが見つかったら、質問にする。
func TestReconsiderMayAsk(t *testing.T) {
	f := newFixture(t)
	f.ai.judgeFn = func(call int) (ai.ReplyJudgment, error) {
		if call == 1 {
			return ai.ReplyJudgment{NeedsReply: false, ReasonKind: ai.ReasonFYI, Reason: "共有のみ", Questions: []string{}}, nil
		}
		return ai.ReplyJudgment{NeedsReply: true, ReasonKind: ai.ReasonRequest, Reason: "依頼", Questions: []string{"期限は？"}}, nil
	}
	f.enable(t)
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)

	got, err := f.svc.Agent.ReconsiderAgentItem(t.Context(), f.only(t).ID)
	require.NoError(t, err)
	assert.Equal(t, "asking", got.Status)
	assert.Equal(t, []string{"期限は？"}, got.Questions)
	_, generates := f.ai.counts()
	assert.Zero(t, generates)
}

// 判断・下書きづくりの失敗は failed（理由の種類だけ）で残り、「やっぱり返信する」でやり直せる。
func TestFailuresAreKeptAndRetryable(t *testing.T) {
	f := newFixture(t)
	f.ai.judgeFn = func(call int) (ai.ReplyJudgment, error) {
		if call == 1 {
			return ai.ReplyJudgment{}, fmt.Errorf("%w: provider said %s", ai.ErrFailed, secretBody)
		}
		return ai.ReplyJudgment{NeedsReply: true, ReasonKind: ai.ReasonRequest, Reason: "依頼", Questions: []string{}}, nil
	}
	f.enable(t)

	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
	it := f.only(t)
	assert.Equal(t, "failed", it.Status)
	assert.Equal(t, "ai_failed", it.ReasonKind, "エラーの種類だけ")
	assert.NotContains(t, it.Reason, secretBody)
	assert.Zero(t, f.items(t).PendingCount)

	got, err := f.svc.Agent.ReconsiderAgentItem(t.Context(), it.ID)
	require.NoError(t, err)
	assert.Equal(t, "ready", got.Status)

	// 下書きづくりの失敗。
	f.ai.generateFn = func(int, ai.ReplyInput) (string, error) { return "", ai.ErrRateLimited }
	f.putInbound(t, f.a, "t2", "m2")
	f.newMail(f.a.ID, "t2", "m2")
	f.waitItems(t, 2)
	var failed string
	for _, x := range f.items(t).Items {
		if x.ThreadID == "t2" {
			assert.Equal(t, "failed", x.Status)
			assert.Equal(t, "rate_limited", x.ReasonKind)
			failed = x.ID
		}
	}
	// 失敗のままの Reconsider（まだ失敗する）はエラーを返し、failed のまま。
	_, err = f.svc.Agent.ReconsiderAgentItem(t.Context(), failed)
	requireCode(t, "RATE_LIMITED", err)
	f.ai.generateFn = nil
	got, err = f.svc.Agent.ReconsiderAgentItem(t.Context(), failed)
	require.NoError(t, err)
	assert.Equal(t, "ready", got.Status)
	f.requireNeverSent(t)
}
