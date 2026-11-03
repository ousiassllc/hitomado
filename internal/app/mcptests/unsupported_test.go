package mcptests

import (
	"encoding/json"
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/capability"
	"github.com/ousiassllc/hitomado/internal/mcpserver"
	"github.com/ousiassllc/hitomado/internal/store"
)

// Android（MCP の機能が無い OS）では、PC で保存した「オン」の設定が DB にあっても待ち受けない（#89）。
func TestMCPNotStartedWhenUnsupportedEvenIfSavedEnabled(t *testing.T) {
	m := newMCPEnvFor(t, capability.Android())
	port := freePort(t)
	raw, err := json.Marshal(mcpserver.Config{Enabled: true, Host: "127.0.0.1", Port: port})
	require.NoError(t, err)
	require.NoError(t, m.Store.SetSetting(t.Context(), store.SettingMCPConfig, string(raw)))

	m.Svc.Start(t.Context())

	c, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err == nil {
		_ = c.Close()
	}
	require.Error(t, err, "MCP の機能が無い OS では待ち受けない")
}
