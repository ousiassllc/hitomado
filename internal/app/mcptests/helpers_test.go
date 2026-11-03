package mcptests

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/capability"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/store"
)

func at(minutes int) time.Time { return apptest.Epoch.Add(time.Duration(minutes) * time.Minute) }

func exhaust(t *testing.T, e *apptest.Env, ids ...string) {
	t.Helper()
	for _, id := range ids {
		require.NoError(t, e.Store.SaveSyncState(t.Context(), store.SyncState{AccountID: id, InboxExhausted: true, InitialSyncDone: true}))
	}
}

func threadIDs(items []dto.ThreadSummary) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ThreadID
	}
	return out
}

func requireCode(t *testing.T, want string, err error) {
	t.Helper()
	require.Error(t, err)
	var ae *apperr.Error
	require.True(t, errors.As(err, &ae), "apperr.Error ではない: %v", err)
	require.Equal(t, want, ae.Code)
}

type mcpEnv struct {
	*apptest.Env
	Svc    *app.MCPService
	LogDir string
}

func newMCPEnv(t *testing.T) *mcpEnv {
	t.Helper()
	return newMCPEnvFor(t, capability.Desktop("windows"))
}

// newMCPEnvFor は OS の機能（capability）を指定して組み立てる。
func newMCPEnvFor(t *testing.T, caps capability.Capabilities) *mcpEnv {
	t.Helper()
	e := apptest.NewEnv(t)
	e.Deps.Capabilities = caps
	dir := filepath.Join(e.Deps.DataDir, app.LogsDirName)
	l, closeLog, err := logging.New(dir, slog.LevelDebug, false, logging.Meta{Version: "t"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = closeLog() })
	e.Deps.Logger = l
	svc := app.NewServices(e.Deps).MCP
	t.Cleanup(svc.Shutdown)
	return &mcpEnv{Env: e, Svc: svc, LogDir: dir}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func (m *mcpEnv) enable(t *testing.T) dto.MCPStatus {
	t.Helper()
	st, err := m.Svc.SetMCPConfig(t.Context(), dto.MCPConfig{Enabled: true, Host: "127.0.0.1", Port: freePort(t)})
	require.NoError(t, err)
	require.True(t, st.Listening, st.Error)
	return st
}

func (m *mcpEnv) issueToken(t *testing.T) string {
	t.Helper()
	res, err := m.Svc.IssueMCPToken(t.Context())
	require.NoError(t, err)
	return res.Token
}

func (m *mcpEnv) logText(t *testing.T) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(m.LogDir, "*.log"))
	require.NoError(t, err)
	var b strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // テストの一時ディレクトリ配下
		require.NoError(t, err)
		b.Write(data)
	}
	return b.String()
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func connect(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	c := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := c.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: endpoint, HTTPClient: &http.Client{Transport: bearerTransport{token}}, DisableStandaloneSSE: true,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func post(t *testing.T, endpoint, authz string) int {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"0"}}}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

func call(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	return res
}

func structured(t *testing.T, res *mcp.CallToolResult, into any) {
	t.Helper()
	require.False(t, res.IsError, "%+v", res.Content)
	b, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, into))
}

// --- 仕様 (1): 既定はオフ ---
