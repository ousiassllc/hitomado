package apptests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
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
	err := svc.DeleteOAuthClient(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.OAuthClientInUse, err)
	_, err = e.Secrets.Get(secrets.OAuthClientSecretKey(a.OAuthClientID))
	assert.NoError(t, err, "使用中の Secret は消さない")

	list, err := svc.ListOAuthClients(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 1, list[0].AccountCount)
}
