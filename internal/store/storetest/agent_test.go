package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store"
)

func agentItem(accountID, thread, msg, status string) store.AgentItem {
	return store.AgentItem{
		AccountID: accountID, ThreadID: thread, MessageID: msg, Status: status,
		Subject: "件名", FromLabel: "Alice", ReasonKind: "request", Reason: "依頼されています",
	}
}

func TestAgentItemRoundTripAndOrder(t *testing.T) {
	ctx := context.Background()
	s, clk := newTestStore(t)
	acct := seedAccount(t, s, "a@example.com")

	first := agentItem(acct.ID, "t1", "m1", store.AgentStatusReady)
	first.DraftID = "d1"
	first.To = []model.Address{{Name: "Alice", Email: "alice@x.example"}}
	created, err := s.CreateAgentItem(ctx, first)
	require.NoError(t, err)
	require.NotEmpty(t, created.ID)

	clk.Advance(time.Minute)
	asking := agentItem(acct.ID, "t2", "m2", store.AgentStatusAsking)
	asking.Questions = []string{"日程は？", "金額は？"}
	second, err := s.CreateAgentItem(ctx, asking)
	require.NoError(t, err)

	got, err := s.GetAgentItem(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, first.To, got.To)
	assert.Equal(t, "d1", got.DraftID)
	assert.Equal(t, []string{}, got.Questions, "空の質問は空配列（null にしない）")

	list, err := s.ListAgentItems(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, second.ID, list[0].ID, "新しい順")
	assert.Equal(t, []string{"日程は？", "金額は？"}, list[0].Questions)

	n, err := s.CountPendingAgentItems(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestAgentItemUniqueKey(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	a := seedAccount(t, s, "a@example.com")
	b := seedAccount(t, s, "b@example.com")

	_, err := s.CreateAgentItem(ctx, agentItem(a.ID, "t1", "m1", store.AgentStatusReady))
	require.NoError(t, err)
	_, err = s.CreateAgentItem(ctx, agentItem(a.ID, "t1", "m1", store.AgentStatusSkipped))
	require.ErrorIs(t, err, store.ErrDuplicate)

	_, err = s.CreateAgentItem(ctx, agentItem(a.ID, "t1", "m2", store.AgentStatusReady))
	require.NoError(t, err, "同じスレッドでもメッセージが違えば別の行")
	_, err = s.CreateAgentItem(ctx, agentItem(b.ID, "t1", "m1", store.AgentStatusReady))
	require.NoError(t, err, "アカウントが違えば別の行")

	ok, err := s.AgentItemExists(ctx, a.ID, "t1", "m1")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = s.AgentItemExists(ctx, a.ID, "t9", "m1")
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = s.CreateAgentItem(ctx, agentItem("no-such-account", "t1", "m1", store.AgentStatusReady))
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = s.CreateAgentItem(ctx, agentItem(a.ID, "t5", "m5", "bogus"))
	require.Error(t, err, "未知の status は DB が拒否する")
}

func TestAgentItemUpdateAndNotFound(t *testing.T) {
	ctx := context.Background()
	s, clk := newTestStore(t)
	acct := seedAccount(t, s, "a@example.com")
	it, err := s.CreateAgentItem(ctx, agentItem(acct.ID, "t1", "m1", store.AgentStatusAsking))
	require.NoError(t, err)

	clk.Advance(time.Hour)
	it.Status, it.DraftID, it.To = store.AgentStatusReady, "d9", []model.Address{{Email: "x@example.com"}}
	updated, err := s.UpdateAgentItem(ctx, it)
	require.NoError(t, err)
	assert.Equal(t, store.AgentStatusReady, updated.Status)
	assert.Equal(t, "d9", updated.DraftID)
	assert.True(t, updated.UpdatedAt.After(updated.CreatedAt), "更新で updated_at が進む")

	_, err = s.UpdateAgentItem(ctx, store.AgentItem{ID: "nope", Status: store.AgentStatusDone})
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = s.GetAgentItem(ctx, "nope")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestAgentItemsPurgeFinishedOnly(t *testing.T) {
	ctx := context.Background()
	s, clk := newTestStore(t)
	acct := seedAccount(t, s, "a@example.com")
	for i, st := range []string{store.AgentStatusDone, store.AgentStatusSkipped, store.AgentStatusFailed, store.AgentStatusReady, store.AgentStatusAsking, store.AgentStatusLater} {
		_, err := s.CreateAgentItem(ctx, agentItem(acct.ID, "t", string(rune('a'+i)), st))
		require.NoError(t, err)
	}
	clk.Advance(31 * 24 * time.Hour)
	fresh, err := s.CreateAgentItem(ctx, agentItem(acct.ID, "t", "fresh", store.AgentStatusDone))
	require.NoError(t, err)

	n, err := s.DeleteFinishedAgentItemsBefore(ctx, clk.Now().Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 3, n, "古い done／skipped／failed だけ消える")

	list, err := s.ListAgentItems(ctx)
	require.NoError(t, err)
	statuses := map[string]bool{}
	for _, it := range list {
		statuses[it.Status] = true
	}
	assert.Len(t, list, 4)
	assert.True(t, statuses[store.AgentStatusReady] && statuses[store.AgentStatusAsking] && statuses[store.AgentStatusLater], "確認待ち・質問待ち・あとではどれだけ古くても残す")
	_, err = s.GetAgentItem(ctx, fresh.ID)
	require.NoError(t, err, "新しい done は残る")
}

func TestAgentItemsCascadeWithAccount(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	a := seedAccount(t, s, "a@example.com")
	b := seedAccount(t, s, "b@example.com")
	_, err := s.CreateAgentItem(ctx, agentItem(a.ID, "t1", "m1", store.AgentStatusReady))
	require.NoError(t, err)
	_, err = s.CreateAgentItem(ctx, agentItem(b.ID, "t1", "m1", store.AgentStatusReady))
	require.NoError(t, err)

	require.NoError(t, s.DeleteAccount(ctx, a.ID))
	list, err := s.ListAgentItems(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, b.ID, list[0].AccountID)
}
