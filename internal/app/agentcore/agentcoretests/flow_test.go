package agentcoretests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store"
)

// オフのとき（既定）は、新着が来ても AI・Gmail の下書き・スレッド取得・行のどれも動かない。
func TestOffDoesNothingOnNewMail(t *testing.T) {
	f := newFixture(t)
	f.ready(t) // AI は使える状態。それでもオフなら何もしない
	f.start(t)

	f.newMail(f.a.ID, "t1", "m1")
	f.stop()

	judges, generates := f.ai.counts()
	assert.Zero(t, judges)
	assert.Zero(t, generates)
	assert.Zero(t, f.Gmail.CallCount("CreateDraft"))
	assert.Zero(t, f.Gmail.CallCount("GetThreadFull"), "スレッドも取得しない")
	assert.Empty(t, f.items(t).Items)
	f.requireNeverSent(t)
}

// オンでも AI が使えない（未設定・未同意）なら、何もしない。
func TestOnButAINotReadyDoesNothing(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAgent, `{"enabled":true}`))
	f.start(t)

	f.newMail(f.a.ID, "t1", "m1") // AI 未設定
	f.configure(t)
	f.newMail(f.a.ID, "t1", "m1")     // 設定済み・同意なし
	f.waitLog(t, "agent: skipped", 2) // 2 件とも、使えない状態として見送られた（ワーカーが処理した）
	f.stop()

	judges, _ := f.ai.counts()
	assert.Zero(t, judges)
	assert.Zero(t, f.Gmail.CallCount("GetThreadFull"))
	assert.Zero(t, f.Gmail.CallCount("CreateDraft"))
	assert.Empty(t, f.items(t).Items)
}

// オフの間に来た新着は、あとでオンにしても処理されない（積まれもしない）。
func TestNewMailWhileOffIsNotProcessedLater(t *testing.T) {
	f := newFixture(t)
	f.ready(t)
	f.start(t)
	f.newMail(f.a.ID, "t1", "m1") // オフ

	_, err := f.svc.Agent.SetAgentEnabled(t.Context(), true, true)
	require.NoError(t, err)
	f.newMail(f.b.ID, "u1", "u-m1")
	f.waitItems(t, 1)
	f.stop()

	judges, _ := f.ai.counts()
	assert.Equal(t, 1, judges, "オンになってから来た 1 通だけ")
	assert.Equal(t, f.b.ID, f.only(t).AccountID)
}

// 返信が要り・質問が無いとき、受信したアカウントの署名つきの返信下書きを作り、確認待ち（ready）にする。送信はしない。
func TestReplyNeededCreatesDraftWithThatAccountsSignature(t *testing.T) {
	f := newFixture(t)
	f.enable(t)

	f.newMail(f.a.ID, "t1", "m1")
	f.newMail(f.b.ID, "u1", "u-m1")
	f.waitItems(t, 2)

	byAccount := map[string]string{}
	for _, it := range f.items(t).Items {
		byAccount[it.AccountID] = it.ID
		assert.Equal(t, "ready", it.Status)
		assert.Equal(t, "request", it.ReasonKind)
		assert.Equal(t, "依頼されています", it.Reason)
		assert.Equal(t, secretSubject, it.Subject)
		assert.Equal(t, secretFrom, it.From)
		assert.NotEmpty(t, it.DraftID)
		assert.Equal(t, []model.Address{{Name: secretFrom, Email: "alice@x.example"}}, it.To, "宛先は下書きの宛先（元の差出人）")
		assert.Empty(t, it.Questions)
		assert.Contains(t, it.DraftPreview, secretDraft)
	}
	assert.Equal(t, 2, f.items(t).PendingCount)

	for _, acct := range []struct {
		id, thread, want, notWant, inReplyTo string
	}{
		{f.a.ID, "t1", "署名A", "署名B", "<m1@example.com>"},
		{f.b.ID, "u1", "署名B", "署名A", "<u-m1@example.com>"},
	} {
		var item string
		for _, it := range f.items(t).Items {
			if it.AccountID == acct.id {
				item = it.DraftID
			}
		}
		d, err := f.Gmail.GetDraft(t.Context(), acct.id, item)
		require.NoError(t, err, "下書きは受信したアカウントの Gmail にある")
		assert.Equal(t, acct.thread, d.ThreadID)
		body := d.Message.BodyHTML
		assert.Contains(t, body, acct.want)
		assert.NotContains(t, body, acct.notWant)
		bodyAt, sigAt := strings.Index(body, secretDraft), strings.Index(body, acct.want)
		require.True(t, bodyAt >= 0 && sigAt > bodyAt, "AI の本文は署名より前: %s", body)
		quoteAt := strings.Index(body, secretBody)
		require.True(t, quoteAt > sigAt, "署名のあとに引用: %s", body)
		assert.Contains(t, string(f.Gmail.DraftRaw[apptest.Key(acct.id, item)]), "In-Reply-To: "+acct.inReplyTo)
	}
	f.requireNeverSent(t)
}

// 返信不要と判断したものは skipped で保存され、下書きは作らない。確認待ちの件数にも入らない。
func TestNoReplySkips(t *testing.T) {
	f := newFixture(t)
	f.ai.judgeFn = func(int) (ai.ReplyJudgment, error) {
		return ai.ReplyJudgment{NeedsReply: false, ReasonKind: ai.ReasonNewsletter, Reason: "メルマガです", Questions: []string{}}, nil
	}
	f.enable(t)

	f.newMail(f.a.ID, "t1", "m1")
	f.waitItems(t, 1)

	it := f.only(t)
	assert.Equal(t, "skipped", it.Status)
	assert.Equal(t, "newsletter", it.ReasonKind)
	assert.Equal(t, "メルマガです", it.Reason)
	assert.Empty(t, it.DraftID)
	assert.Zero(t, f.items(t).PendingCount)
	_, generates := f.ai.counts()
	assert.Zero(t, generates, "本文は作らない")
	assert.Zero(t, f.Gmail.CallCount("CreateDraft"))
	f.requireNeverSent(t)
}
