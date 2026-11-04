package siwctests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

func TestRemoveAccount(t *testing.T) {
	e := twoAccounts(t, nil, nil)

	assert.ErrorIs(t, e.m.RemoveAccount(t.Context(), secondClient), siwc.ErrAccountInUse, "ログイン中は消せない")
	assert.ErrorIs(t, e.m.RemoveAccount(t.Context(), "oaiapp_UNKNOWN"), siwc.ErrUnknownAccount)

	_, err := e.m.SignOut(t.Context(), secondClient)
	require.NoError(t, err)
	require.NoError(t, e.m.RemoveAccount(t.Context(), secondClient))
	reg := e.reg(t)
	require.Len(t, reg.Accounts, 1)
	assert.Equal(t, issuedClient, reg.Accounts[0].ClientID)
	assert.Empty(t, reg.Active, "使用中だったアカウントを消したら、使用中は無し")
	got, err := e.sec.Get(tokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, secretRefresh, got, "もう片方のトークンは残る")
	assert.NotEmpty(t, reg.HostID, "ホスト ID は残す")
}

func TestLongTokensOfTwoAccountsDoNotCollide(t *testing.T) {
	sec := limitedSecrets{Memory: secrets.NewMemory(), max: 2560}
	e := newEnv(t, sec, nil)
	e.f.accessToken = strings.Repeat("A", 6500)
	e.signIn(t)
	e.f.mu.Lock()
	e.f.accessToken = strings.Repeat("B", 6500)
	e.f.issueQueue = []string{secondClient}
	e.f.mu.Unlock()
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.NoError(t, err)

	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("B", 6500), tok)
	require.NoError(t, e.m.SelectAccount(t.Context(), issuedClient))
	tok, err = e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("A", 6500), tok, "断片が別のアカウントのものと混ざらない")

	_, err = e.m.SignOut(t.Context(), secondClient)
	require.NoError(t, err)
	_, err = sec.Get(secondTokenKey("access") + "#0")
	assert.ErrorIs(t, err, secrets.ErrNotFound)
	tok, err = e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("A", 6500), tok)
}
