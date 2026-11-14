package agentcoretests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// twoDrafts は a・b それぞれに ready の項目を作って、ワーカーを止める。a の項目を返す。
func (f *fixture) twoDrafts(t *testing.T) (itemA string, draftA string) {
	t.Helper()
	f.enable(t)
	f.newMail(f.a.ID, "t1", "m1")
	f.newMail(f.b.ID, "u1", "u-m1")
	f.waitItems(t, 2)
	f.stop()
	for _, it := range f.items(t).Items {
		if it.AccountID == f.a.ID {
			return it.ID, it.DraftID
		}
	}
	require.FailNow(t, "a の項目が無い")
	return "", ""
}

// 下書きが Gmail から無くなった（削除された・スレッド画面から送信された）ready の項目は、確認済み（done）に倒して保存し、
// 一覧に出さない（確認待ちの件数も下がる）。agent:updated も発行する。ほかの項目は残る。
func TestGoneDraftFinishesTheItemAndLeavesTheList(t *testing.T) {
	f := newFixture(t)
	itemA, draftA := f.twoDrafts(t)
	delete(f.Gmail.Drafts, apptest.Key(f.a.ID, draftA))
	before := len(f.Events.Named(events.AgentUpdated))

	o, err := f.restart().Agent.ListAgentItems(t.Context())
	require.NoError(t, err)
	require.Len(t, o.Items, 1)
	assert.Equal(t, f.b.ID, o.Items[0].AccountID)
	assert.Equal(t, 1, o.PendingCount)
	saved, err := f.Store.GetAgentItem(t.Context(), itemA)
	require.NoError(t, err)
	assert.Equal(t, store.AgentStatusDone, saved.Status)

	updated := f.Events.Named(events.AgentUpdated)
	require.Greater(t, len(updated), before)
	payload, ok := updated[len(updated)-1].Payload.(events.AgentUpdatedPayload)
	require.True(t, ok)
	assert.Equal(t, 1, payload.PendingCount)
	f.requireNeverSent(t)
}

// 「あとで」の項目も、下書きが無くなっていたら done に倒れる。単一項目の返却（操作の結果）でも同じ。
func TestGoneDraftFinishesLaterItemAndSingleItemResult(t *testing.T) {
	f := newFixture(t)
	itemA, draftA := f.twoDrafts(t)
	_, err := f.svc.Agent.DeferAgentItem(t.Context(), itemA)
	require.NoError(t, err)
	delete(f.Gmail.Drafts, apptest.Key(f.a.ID, draftA))

	o, err := f.restart().Agent.ListAgentItems(t.Context())
	require.NoError(t, err)
	require.Len(t, o.Items, 1)
	saved, err := f.Store.GetAgentItem(t.Context(), itemA)
	require.NoError(t, err)
	assert.Equal(t, store.AgentStatusDone, saved.Status)

	// 単一項目: 下書きが無いとき、操作の結果に done が返り、保存も done。
	f2 := newFixture(t)
	item2, draft2 := f2.twoDrafts(t)
	delete(f2.Gmail.Drafts, apptest.Key(f2.a.ID, draft2))
	got, err := f2.restart().Agent.DeferAgentItem(t.Context(), item2)
	require.NoError(t, err)
	assert.Equal(t, "done", got.Status)
	assert.Empty(t, got.DraftPreview)
}

// 取得の失敗（NotFound 以外）は、5 分間は覚えて Gmail を叩き直さない。時間が過ぎたら取り直す。失敗では項目を倒さない。
func TestFailedPreviewIsRememberedForFiveMinutes(t *testing.T) {
	f := newFixture(t)
	f.twoDrafts(t)
	f.Gmail.Err["GetDraft"] = gmail.ErrRateLimited
	svc := f.restart()

	for range 3 {
		o, err := svc.Agent.ListAgentItems(t.Context())
		require.NoError(t, err)
		require.Len(t, o.Items, 2)
	}
	assert.Equal(t, 2, f.Gmail.CallCount("GetDraft"), "2 件を 1 回ずつ。以降は取り直さない")

	f.Clock.Advance(5*time.Minute + time.Second)
	_, err := svc.Agent.ListAgentItems(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 4, f.Gmail.CallCount("GetDraft"), "時間が過ぎたら取り直す")
	assert.Equal(t, 2, f.items(t).PendingCount)
}

// オフのときは、一覧で Gmail の下書きを一切取りに行かない（項目は返すがプレビューは空）。
func TestListItemsDoesNotTouchGmailWhileOff(t *testing.T) {
	f := newFixture(t)
	f.twoDrafts(t)
	_, err := f.svc.Agent.SetAgentEnabled(t.Context(), false, false)
	require.NoError(t, err)

	o, err := f.restart().Agent.ListAgentItems(t.Context())
	require.NoError(t, err)
	require.Len(t, o.Items, 2)
	for _, it := range o.Items {
		assert.Empty(t, it.DraftPreview)
	}
	assert.Zero(t, f.Gmail.CallCount("GetDraft"))
}

// 確認済み（done）の項目はパネルが描かないので、プレビューのために下書きを取りに行かない。
func TestDoneItemsAreNotPreviewed(t *testing.T) {
	f := newFixture(t)
	itemA, _ := f.twoDrafts(t)
	_, err := f.svc.Agent.ConfirmAgentItem(t.Context(), itemA)
	require.NoError(t, err)

	_, err = f.restart().Agent.ListAgentItems(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, f.Gmail.CallCount("GetDraft"), "ready の b の分だけ")
}
