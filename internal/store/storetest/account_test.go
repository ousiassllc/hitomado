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

func TestOAuthClientLifecycle(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()

	c := store.OAuthClient{ID: store.NewID(), Name: "A社", ClientID: "cid-a"}
	require.NoError(t, s.CreateOAuthClient(ctx, c))
	assert.ErrorIs(t, s.CreateOAuthClient(ctx, store.OAuthClient{ID: store.NewID(), Name: "dup", ClientID: "cid-a"}), store.ErrDuplicate)
	require.Error(t, s.CreateOAuthClient(ctx, store.OAuthClient{ID: store.NewID(), Name: "", ClientID: "cid-b"}), "空の name は CHECK で拒否")

	clk.Advance(time.Second)
	got, err := s.UpdateOAuthClient(ctx, c.ID, "A社 Workspace", "cid-a2")
	require.NoError(t, err)
	assert.Equal(t, "A社 Workspace", got.Name)
	assert.Equal(t, "cid-a2", got.ClientID)
	assert.True(t, got.UpdatedAt.After(got.CreatedAt))

	_, err = s.UpdateOAuthClient(ctx, "missing", "x", "y")
	assert.ErrorIs(t, err, store.ErrNotFound)
	other := store.OAuthClient{ID: store.NewID(), Name: "B", ClientID: "cid-b"}
	require.NoError(t, s.CreateOAuthClient(ctx, other))
	_, err = s.UpdateOAuthClient(ctx, other.ID, "B", "cid-a2")
	assert.ErrorIs(t, err, store.ErrDuplicate)

	_, err = s.GetOAuthClient(ctx, "missing")
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.ErrorIs(t, s.DeleteOAuthClient(ctx, "missing"), store.ErrNotFound)
	require.NoError(t, s.DeleteOAuthClient(ctx, other.ID))
}

func TestOAuthClientInUseIsRestricted(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")

	assert.ErrorIs(t, s.DeleteOAuthClient(ctx, a.OAuthClientID), store.ErrOAuthClientUse)
	clients, err := s.ListOAuthClients(ctx)
	require.NoError(t, err)
	require.Len(t, clients, 1)
	assert.Equal(t, 1, clients[0].AccountCount)

	// アカウントを消せばクライアントも消せる。
	require.NoError(t, s.DeleteAccount(ctx, a.ID))
	require.NoError(t, s.DeleteOAuthClient(ctx, a.OAuthClientID))
	clients, err = s.ListOAuthClients(ctx)
	require.NoError(t, err)
	assert.Empty(t, clients)
	assert.NotNil(t, clients)
}

func TestCreateAccount(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	oc := store.OAuthClient{ID: store.NewID(), Name: "c", ClientID: "cid"}
	require.NoError(t, s.CreateOAuthClient(ctx, oc))

	id := store.NewID()
	require.NoError(t, s.CreateAccount(ctx, store.Account{
		ID: id, OAuthClientID: oc.ID, Email: "  User@Example.COM ", DisplayName: "U", Color: "blue", NotifyEnabled: true,
	}))
	got, err := s.GetAccount(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", got.Email)
	assert.Equal(t, store.StatusActive, got.Status, "status 未指定は active")
	assert.True(t, got.NotifyEnabled)
	assert.Equal(t, "", got.SignatureHTML)

	// account_sync_state も作成される（既定値）。
	st, err := s.GetSyncState(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, store.SyncState{AccountID: id}, st)

	// 大文字小文字違いのメールは重複。重複時は account_sync_state も残らない。
	dup := store.NewID()
	err = s.CreateAccount(ctx, store.Account{ID: dup, OAuthClientID: oc.ID, Email: "USER@example.com", DisplayName: "x", Color: "red"})
	assert.ErrorIs(t, err, store.ErrDuplicate)
	assert.Equal(t, 1, count(t, s, `SELECT COUNT(*) FROM account_sync_state`))

	err = s.CreateAccount(ctx, store.Account{ID: store.NewID(), OAuthClientID: "no-such-client", Email: "z@example.com", DisplayName: "x", Color: "red"})
	assert.ErrorIs(t, err, store.ErrNotFound)
	require.Error(t, s.CreateAccount(ctx, store.Account{ID: store.NewID(), OAuthClientID: oc.ID, Email: " ", DisplayName: "x", Color: "red"}))
	require.Error(t, s.CreateAccount(ctx, store.Account{ID: store.NewID(), OAuthClientID: oc.ID, Email: "s@example.com", DisplayName: "x", Color: "red", Status: "bogus"}),
		"不正な status は CHECK で拒否")
}

func TestAccountUpdates(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")

	clk.Advance(time.Second)
	name, color, notify := "新しい名前", "teal", false
	got, err := s.UpdateAccount(ctx, a.ID, store.AccountPatch{DisplayName: &name, Color: &color, NotifyEnabled: &notify})
	require.NoError(t, err)
	assert.Equal(t, name, got.DisplayName)
	assert.Equal(t, color, got.Color)
	assert.False(t, got.NotifyEnabled)
	assert.True(t, got.UpdatedAt.After(a.UpdatedAt))

	// nil のフィールドは変更されない。
	got, err = s.UpdateAccount(ctx, a.ID, store.AccountPatch{})
	require.NoError(t, err)
	assert.Equal(t, name, got.DisplayName)
	_, err = s.UpdateAccount(ctx, "missing", store.AccountPatch{})
	assert.ErrorIs(t, err, store.ErrNotFound)

	require.NoError(t, s.SetAccountStatus(ctx, a.ID, store.StatusNeedsReauth, "再認証が必要"))
	got, err = s.GetAccount(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, store.StatusNeedsReauth, got.Status)
	assert.Equal(t, "再認証が必要", got.StatusMessage)
	require.NoError(t, s.SetAccountStatus(ctx, a.ID, store.StatusActive, ""))
	got, _ = s.GetAccount(ctx, a.ID)
	assert.Equal(t, "", got.StatusMessage)
	assert.ErrorIs(t, s.SetAccountStatus(ctx, "missing", store.StatusActive, ""), store.ErrNotFound)
	require.Error(t, s.SetAccountStatus(ctx, a.ID, "syncing", ""), "syncing は DB に保存しない")

	got, err = s.SetSignature(ctx, a.ID, "<p>署名</p>")
	require.NoError(t, err)
	assert.Equal(t, "<p>署名</p>", got.SignatureHTML)
	_, err = s.SetSignature(ctx, "missing", "x")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestSyncStateRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")

	want := store.SyncState{
		AccountID: a.ID, HistoryID: "123", InboxPageToken: "tok", InboxExhausted: true, InitialSyncDone: true,
		LastSyncedAt: time.UnixMilli(1730000000000).UTC(), LastError: "boom",
	}
	require.NoError(t, s.SaveSyncState(ctx, want))
	got, err := s.GetSyncState(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// 空値は NULL に戻る（history_id を NULL に戻してリセット同期）。
	require.NoError(t, s.SaveSyncState(ctx, store.SyncState{AccountID: a.ID}))
	got, _ = s.GetSyncState(ctx, a.ID)
	assert.Equal(t, store.SyncState{AccountID: a.ID}, got)

	assert.ErrorIs(t, s.SaveSyncState(ctx, store.SyncState{AccountID: "missing"}), store.ErrNotFound)
	_, err = s.GetSyncState(ctx, "missing")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestReorderAccounts(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	b := seedAccount(t, s, "b@example.com")
	c := seedAccount(t, s, "c@example.com")
	ids := func() []string {
		list, err := s.ListAccounts(ctx)
		require.NoError(t, err)
		out := make([]string, 0, len(list))
		for _, x := range list {
			out = append(out, x.ID)
		}
		return out
	}
	assert.Equal(t, []string{a.ID, b.ID, c.ID}, ids(), "作成順に末尾へ追加される")

	require.NoError(t, s.ReorderAccounts(ctx, []string{c.ID, a.ID, b.ID}))
	assert.Equal(t, []string{c.ID, a.ID, b.ID}, ids())

	for name, bad := range map[string][]string{
		"不足":     {a.ID, b.ID},
		"過剰":     {a.ID, b.ID, c.ID, "x"},
		"重複":     {a.ID, a.ID, b.ID},
		"未知の ID": {a.ID, b.ID, "x"},
		"空":      {},
	} {
		require.Error(t, s.ReorderAccounts(ctx, bad), name)
	}
	assert.Equal(t, []string{c.ID, a.ID, b.ID}, ids(), "失敗時は並びが変わらない")
}

func TestDeleteAccountCascades(t *testing.T) {
	s, clk := newTestStore(t)
	ctx := context.Background()
	a := seedAccount(t, s, "a@example.com")
	b := seedAccount(t, s, "b@example.com")
	now := clk.Now()
	for _, acc := range []store.Account{a, b} {
		require.NoError(t, s.ReplaceLabels(ctx, acc.ID, []store.Label{{ID: "Label_1", Name: "x", Type: "user"}}))
		apply(t, s, snap(acc.ID, "t1", now, "INBOX", "Label_1"))
		require.NoError(t, s.AddReceivedContacts(ctx, acc.ID, []model.Address{{Email: "p@example.com"}}, now))
	}

	require.NoError(t, s.DeleteAccount(ctx, a.ID))
	assert.ErrorIs(t, s.DeleteAccount(ctx, a.ID), store.ErrNotFound)
	for _, table := range []string{"account_sync_state", "labels", "threads", "messages", "message_labels", "contacts"} {
		other := count(t, s, `SELECT COUNT(*) FROM `+table+` WHERE account_id = ?`, b.ID)
		assert.Positive(t, other, table)
		assert.Equal(t, other, count(t, s, `SELECT COUNT(*) FROM `+table), table+" は他アカウント分だけ残る")
		assert.Equal(t, 0, count(t, s, `SELECT COUNT(*) FROM `+table+` WHERE account_id = ?`, a.ID), table)
	}
}
