package apptests

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

func TestOAuthClientCRUD(t *testing.T) {
	svc, e := newAccountSvc(t)
	ctx := t.Context()

	c, err := svc.AddOAuthClient(ctx, " Personal ", " cid-1 ", "sec-1")
	require.NoError(t, err)
	assert.Equal(t, "Personal", c.Name)
	assert.Equal(t, "cid-1", c.ClientID)
	assert.True(t, c.HasSecret)
	got, err := e.Secrets.Get(secrets.OAuthClientSecretKey(c.ID))
	require.NoError(t, err)
	assert.Equal(t, "sec-1", got)

	_, err = svc.AddOAuthClient(ctx, "Other", "cid-1", "sec-x")
	_ = requireCode(t, apperr.Validation, err)
	_, err = svc.AddOAuthClient(ctx, "", "cid-2", "sec")
	_ = requireCode(t, apperr.Validation, err)
	_, err = svc.AddOAuthClient(ctx, "n", "cid-2", "  ")
	_ = requireCode(t, apperr.Validation, err)

	// secret 空は更新しない
	u, err := svc.UpdateOAuthClient(ctx, c.ID, "Renamed", "cid-1b", "")
	require.NoError(t, err)
	assert.Equal(t, "Renamed", u.Name)
	assert.Equal(t, "cid-1b", u.ClientID)
	got, _ = e.Secrets.Get(secrets.OAuthClientSecretKey(c.ID))
	assert.Equal(t, "sec-1", got)
	_, err = svc.UpdateOAuthClient(ctx, c.ID, "Renamed", "cid-1b", "sec-2")
	require.NoError(t, err)
	got, _ = e.Secrets.Get(secrets.OAuthClientSecretKey(c.ID))
	assert.Equal(t, "sec-2", got)

	list, err := svc.ListOAuthClients(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.True(t, list[0].HasSecret)

	require.NoError(t, e.Secrets.Delete(secrets.OAuthClientSecretKey(c.ID)))
	list, _ = svc.ListOAuthClients(ctx)
	assert.False(t, list[0].HasSecret)

	require.NoError(t, svc.DeleteOAuthClient(ctx, c.ID))
	list, err = svc.ListOAuthClients(ctx)
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Empty(t, list)
}

func TestDeleteOAuthClientInUse(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	// 使用中の判定より先に Secret を削除しようとすると、この Delete の失敗（INTERNAL）が先に返る。
	fs := &apptest.FailingSecrets{Store: e.Secrets}
	fs.FailDelete(errors.New("keyring unavailable"))
	e.Deps.Secrets = fs

	err := svc.DeleteOAuthClient(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.OAuthClientInUse, err)
	_, err = e.Secrets.Get(secrets.OAuthClientSecretKey(a.OAuthClientID))
	assert.NoError(t, err, "使用中の Secret は消さない")

	list, err := svc.ListOAuthClients(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 1, list[0].AccountCount)
}

func TestUpdateOAuthClientSecretSaveFailureKeepsClient(t *testing.T) {
	svc, e := newAccountSvc(t)
	ctx := t.Context()
	c, err := svc.AddOAuthClient(ctx, "Personal", "cid-1", "sec-1")
	require.NoError(t, err)

	fs := &apptest.FailingSecrets{Store: e.Secrets}
	fs.FailSet(errors.New("keyring unavailable"))
	e.Deps.Secrets = fs

	_, err = svc.UpdateOAuthClient(ctx, c.ID, "Renamed", "cid-2", "sec-2")
	require.Error(t, err)

	got, err := e.Store.GetOAuthClient(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "Personal", got.Name)
	assert.Equal(t, "cid-1", got.ClientID)
	sec, err := e.Secrets.Get(secrets.OAuthClientSecretKey(c.ID))
	require.NoError(t, err)
	assert.Equal(t, "sec-1", sec)
}

func TestUpdateOAuthClientDBFailureRestoresSecret(t *testing.T) {
	svc, e := newAccountSvc(t)
	ctx := t.Context()
	c, err := svc.AddOAuthClient(ctx, "A", "cid-1", "sec-1")
	require.NoError(t, err)
	_, err = svc.AddOAuthClient(ctx, "B", "cid-2", "sec-b")
	require.NoError(t, err)

	// Client ID 重複で DB 更新が失敗したら、先に保存した新 Secret は旧値へ戻る。
	_, err = svc.UpdateOAuthClient(ctx, c.ID, "A", "cid-2", "sec-new")
	_ = requireCode(t, apperr.Validation, err)

	sec, err := e.Secrets.Get(secrets.OAuthClientSecretKey(c.ID))
	require.NoError(t, err)
	assert.Equal(t, "sec-1", sec)
	got, err := e.Store.GetOAuthClient(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "cid-1", got.ClientID)
}

func TestDeleteOAuthClientSecretDeleteFailureKeepsClient(t *testing.T) {
	svc, e := newAccountSvc(t)
	ctx := t.Context()
	c, err := svc.AddOAuthClient(ctx, "Personal", "cid-1", "sec-1")
	require.NoError(t, err)

	fs := &apptest.FailingSecrets{Store: e.Secrets}
	fs.FailDelete(errors.New("keyring unavailable"))
	e.Deps.Secrets = fs

	require.Error(t, svc.DeleteOAuthClient(ctx, c.ID))

	_, err = e.Store.GetOAuthClient(ctx, c.ID)
	require.NoError(t, err, "Secret を消せなかったときは DB 行を残す")
	sec, err := e.Secrets.Get(secrets.OAuthClientSecretKey(c.ID))
	require.NoError(t, err)
	assert.Equal(t, "sec-1", sec)

	fs.FailDelete(nil)
	require.NoError(t, svc.DeleteOAuthClient(ctx, c.ID))
	_, err = e.Secrets.Get(secrets.OAuthClientSecretKey(c.ID))
	assert.ErrorIs(t, err, secrets.ErrNotFound)
}
