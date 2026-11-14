package agentcoretests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// 利用者の操作: あとで → 確認済み。存在しない ID は NOT_FOUND。状態に合わない操作は VALIDATION。
func TestDeferConfirmAndNotFound(t *testing.T) {
	f := newFixture(t)
	f.enable(t)
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
	it := f.only(t)

	later, err := f.svc.Agent.DeferAgentItem(t.Context(), it.ID)
	require.NoError(t, err)
	assert.Equal(t, "later", later.Status)
	assert.Equal(t, it.DraftID, later.DraftID, "下書きは残る")
	assert.Zero(t, f.items(t).PendingCount)
	_, err = f.svc.Agent.DeferAgentItem(t.Context(), it.ID)
	requireCode(t, "VALIDATION", err)

	done, err := f.svc.Agent.ConfirmAgentItem(t.Context(), it.ID)
	require.NoError(t, err)
	assert.Equal(t, "done", done.Status)
	again, err := f.svc.Agent.ConfirmAgentItem(t.Context(), it.ID)
	require.NoError(t, err, "確認済みの確認は冪等")
	assert.Equal(t, "done", again.Status)

	for name, call := range map[string]func() error{
		"answer": func() error {
			_, err := f.svc.Agent.AnswerAgentQuestions(t.Context(), "nope", []string{"a"})
			return err
		},
		"defer":      func() error { _, err := f.svc.Agent.DeferAgentItem(t.Context(), "nope"); return err },
		"confirm":    func() error { _, err := f.svc.Agent.ConfirmAgentItem(t.Context(), "nope"); return err },
		"reconsider": func() error { _, err := f.svc.Agent.ReconsiderAgentItem(t.Context(), "nope"); return err },
	} {
		t.Run(name, func(t *testing.T) { requireCode(t, "NOT_FOUND", call()) })
	}
}

// 質問待ちのものも「あとで」にでき、答えられる（質問は残る）。下書きの無いものは確認済みにできない。
func TestDeferredQuestionCanStillBeAnswered(t *testing.T) {
	f := newFixture(t)
	f.ai.judgeFn = func(int) (ai.ReplyJudgment, error) {
		return ai.ReplyJudgment{NeedsReply: true, ReasonKind: ai.ReasonQuestion, Reason: "質問", Questions: []string{"金額は？"}}, nil
	}
	f.enable(t)
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
	it := f.only(t)

	_, err := f.svc.Agent.ConfirmAgentItem(t.Context(), it.ID)
	requireCode(t, "VALIDATION", err)
	later, err := f.svc.Agent.DeferAgentItem(t.Context(), it.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"金額は？"}, later.Questions)
	got, err := f.svc.Agent.AnswerAgentQuestions(t.Context(), it.ID, []string{"5000 円"})
	require.NoError(t, err)
	assert.Equal(t, "ready", got.Status)
}

// 項目はアカウント名・アドレスつきで新しい順に返り、null にしない。
func TestListItemsIsNewestFirstWithAccountInfo(t *testing.T) {
	f := newFixture(t)
	o := f.items(t)
	assert.NotNil(t, o.Items)
	assert.Zero(t, o.PendingCount)

	f.enable(t)
	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)
	f.Clock.Advance(1_000_000_000)
	f.newMail(f.b.ID, "u1", "u-m1")
	f.waitItems(t, 2)

	items := f.items(t).Items
	require.Len(t, items, 2)
	assert.Equal(t, f.b.ID, items[0].AccountID, "新しい順")
	assert.Equal(t, f.b.Email, items[0].AccountEmail)
	assert.Equal(t, "name "+f.b.Email, items[0].AccountName)
	assert.Equal(t, "u1", items[0].ThreadID)
	assert.GreaterOrEqual(t, items[0].UpdatedAt, items[1].UpdatedAt)
	assert.NotNil(t, items[0].Questions)
}
