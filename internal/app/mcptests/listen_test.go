package mcptests

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/mcpserver"
)

func TestMCPIsOffByDefault(t *testing.T) {
	m := newMCPEnv(t)
	m.Svc.Start(t.Context())
	st, err := m.Svc.GetMCPStatus(t.Context())
	require.NoError(t, err)
	assert.False(t, st.Enabled, "設定が無ければオフ")
	assert.False(t, st.Listening, "オフなら待ち受けない")
	assert.Equal(t, mcpserver.DefaultHost, st.Host)
	assert.Equal(t, mcpserver.DefaultPort, st.Port)
	assert.False(t, st.HasToken)
}

func TestMCPListensOnlyWhileEnabled(t *testing.T) {
	m := newMCPEnv(t)
	port := freePort(t)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	dial := func() error {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			_ = c.Close()
		}
		return err
	}
	require.Error(t, dial(), "オンにする前は待ち受けない")

	st, err := m.Svc.SetMCPConfig(t.Context(), dto.MCPConfig{Enabled: true, Host: "127.0.0.1", Port: port})
	require.NoError(t, err)
	assert.True(t, st.Listening)
	require.NoError(t, dial())

	st, err = m.Svc.SetMCPConfig(t.Context(), dto.MCPConfig{Enabled: false, Host: "127.0.0.1", Port: port})
	require.NoError(t, err)
	assert.False(t, st.Listening)
	require.Error(t, dial(), "オフに戻したら閉じる")
}

func TestMCPStartHonorsSavedConfig(t *testing.T) {
	m := newMCPEnv(t)
	st := m.enable(t)
	m.Svc.Shutdown()
	cur, err := m.Svc.GetMCPStatus(t.Context())
	require.NoError(t, err)
	assert.False(t, cur.Listening)

	m.Svc.Start(t.Context())
	cur, err = m.Svc.GetMCPStatus(t.Context())
	require.NoError(t, err)
	assert.True(t, cur.Listening)
	assert.Equal(t, st.Endpoint, cur.Endpoint)
}

// --- 仕様 (1): トークン必須（トークンなしの接続は拒否） ---

func TestMCPNonLoopbackNeedsExplicitAllow(t *testing.T) {
	m := newMCPEnv(t)
	_, err := m.Svc.SetMCPConfig(t.Context(), dto.MCPConfig{Enabled: true, Host: "0.0.0.0", Port: freePort(t)})
	requireCode(t, apperr.Validation, err)
	st, err := m.Svc.GetMCPStatus(t.Context())
	require.NoError(t, err)
	assert.False(t, st.Enabled, "拒否した設定は保存しない")

	_, err = m.Svc.SetMCPConfig(t.Context(), dto.MCPConfig{Enabled: true, Host: "127.0.0.1", Port: 80})
	requireCode(t, apperr.Validation, err)
}

func TestMCPListenFailureIsReportedNotFatal(t *testing.T) {
	m := newMCPEnv(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port
	st, err := m.Svc.SetMCPConfig(t.Context(), dto.MCPConfig{Enabled: true, Host: "127.0.0.1", Port: port})
	require.NoError(t, err)
	assert.True(t, st.Enabled)
	assert.False(t, st.Listening)
	assert.NotEmpty(t, st.Error)
}

// --- 仕様 (2): 送信するツールは無い ---
