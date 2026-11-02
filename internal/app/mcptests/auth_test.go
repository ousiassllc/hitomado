package mcptests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
)

func TestMCPRejectsConnectionsWithoutValidToken(t *testing.T) {
	m := newMCPEnv(t)
	st := m.enable(t)
	assert.False(t, st.HasToken)

	assert.Equal(t, http.StatusUnauthorized, post(t, st.Endpoint, ""), "ヘッダなし")
	assert.Equal(t, http.StatusUnauthorized, post(t, st.Endpoint, "Bearer anything"), "トークン未発行のときは何を付けても拒否")

	token := m.issueToken(t)
	assert.Equal(t, http.StatusUnauthorized, post(t, st.Endpoint, ""), "発行後もヘッダなしは拒否")
	assert.Equal(t, http.StatusUnauthorized, post(t, st.Endpoint, "Bearer "+token+"x"), "違うトークン")
	assert.Equal(t, http.StatusUnauthorized, post(t, st.Endpoint, "Basic "+token), "Bearer 以外の方式")
	assert.Equal(t, http.StatusOK, post(t, st.Endpoint, "Bearer "+token), "正しいトークンは通る")

	log := m.logText(t)
	assert.Contains(t, log, "mcp connection rejected", "拒否した接続は記録する")
	assert.NotContains(t, log, token, "トークンの値はログに出さない")
}

func TestMCPTokenRotateAndRevoke(t *testing.T) {
	m := newMCPEnv(t)
	st := m.enable(t)
	old := m.issueToken(t)
	assert.Equal(t, http.StatusOK, post(t, st.Endpoint, "Bearer "+old))

	fresh := m.issueToken(t)
	assert.NotEqual(t, old, fresh)
	assert.Equal(t, http.StatusUnauthorized, post(t, st.Endpoint, "Bearer "+old), "再発行で古いトークンは無効")
	assert.Equal(t, http.StatusOK, post(t, st.Endpoint, "Bearer "+fresh))

	status, err := m.Svc.RevokeMCPToken(t.Context())
	require.NoError(t, err)
	assert.False(t, status.HasToken)
	assert.Equal(t, http.StatusUnauthorized, post(t, st.Endpoint, "Bearer "+fresh), "無効化したら拒否")
}

func TestMCPTokenLivesOnlyInCredentialStore(t *testing.T) {
	m := newMCPEnv(t)
	m.enable(t)
	token := m.issueToken(t)
	stored, err := m.Secrets.Get(secrets.MCPTokenKey())
	require.NoError(t, err)
	assert.Equal(t, token, stored)

	raw, ok, err := m.Store.GetSetting(t.Context(), "mcp.config")
	require.NoError(t, err)
	require.True(t, ok)
	assert.NotContains(t, raw, token)
	st, err := m.Svc.GetMCPStatus(t.Context())
	require.NoError(t, err)
	b, _ := json.Marshal(st)
	assert.NotContains(t, string(b), token)
	assert.True(t, st.HasToken)
}
