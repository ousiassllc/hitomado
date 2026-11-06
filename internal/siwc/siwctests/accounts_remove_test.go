package siwctests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/siwc/siwctest"
)

func TestRemoveAccount(t *testing.T) {
	e := twoAccounts(t, nil, nil)

	assert.ErrorIs(t, e.M.RemoveAccount(t.Context(), secondClient), siwc.ErrAccountInUse, "ログイン中は消せない")
	assert.ErrorIs(t, e.M.RemoveAccount(t.Context(), "oaiapp_UNKNOWN"), siwc.ErrUnknownAccount)

	_, err := e.M.SignOut(t.Context(), secondClient)
	require.NoError(t, err)
	require.NoError(t, e.M.RemoveAccount(t.Context(), secondClient))
	reg := e.Reg(t)
	require.Len(t, reg.Accounts, 1)
	assert.Equal(t, siwctest.IssuedClient, reg.Accounts[0].ClientID)
	assert.Empty(t, reg.Active, "使用中だったアカウントを消したら、使用中は無し")
	got, err := e.Sec.Get(siwctest.TokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretRefresh, got, "もう片方のトークンは残る")
	assert.NotEmpty(t, reg.HostID, "ホスト ID は残す")
}

func TestLongTokensOfTwoAccountsDoNotCollide(t *testing.T) {
	sec := siwctest.LimitedSecrets{Memory: secrets.NewMemory(), Max: 2560}
	e := siwctest.NewEnv(t, sec, nil)
	e.F.AccessToken = strings.Repeat("A", 6500)
	e.SignIn(t)
	e.F.Mu.Lock()
	e.F.AccessToken = strings.Repeat("B", 6500)
	e.F.IssueQueue = []string{secondClient}
	e.F.Mu.Unlock()
	_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.NoError(t, err)

	tok, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("B", 6500), tok)
	require.NoError(t, e.M.SelectAccount(t.Context(), siwctest.IssuedClient))
	tok, err = e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("A", 6500), tok, "断片が別のアカウントのものと混ざらない")

	_, err = e.M.SignOut(t.Context(), secondClient)
	require.NoError(t, err)
	_, err = sec.Get(secondTokenKey("access") + "#0")
	assert.ErrorIs(t, err, secrets.ErrNotFound)
	tok, err = e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("A", 6500), tok)
}
