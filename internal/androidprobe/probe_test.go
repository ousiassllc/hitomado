//go:build android && selfcheck

// Package androidprobe は Android 試作（#81）の実機・エミュレータ向けプローブ。
// 本番コードには含まれない（android ビルドタグのテストのみ）。結果は t.Log に出す。
package androidprobe_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/mcpserver"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

// 本物の go-keyring（zalando）が Android で使えるか。モックにしない。
func TestProbeKeyringReal(t *testing.T) {
	s := secrets.NewKeyring()
	err := s.Set("hitomado/probe/1", "dummy-value")
	t.Logf("keyring Set: err=%v", err)
	_, gerr := s.Get("hitomado/probe/1")
	t.Logf("keyring Get: err=%v", gerr)
	require.Error(t, err, "Android には Secret Service（D-Bus）が無いので失敗する想定。成功したら表の記述を直す")
}

// os.UserConfigDir（run.go の dataDir が使う）が Android で解決できるか。
func TestProbeDataDir(t *testing.T) {
	dir, err := os.UserConfigDir()
	t.Logf("UserConfigDir=%q err=%v HOME=%q XDG_CONFIG_HOME=%q", dir, err, os.Getenv("HOME"), os.Getenv("XDG_CONFIG_HOME"))
	t.Logf("TempDir=%q", os.TempDir())
}

// modernc.org/sqlite: store.Open（マイグレーション込み）と書き込み・再オープン。
func TestProbeSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe.db")
	ctx := context.Background()
	st, err := store.Open(ctx, path)
	require.NoError(t, err)
	require.NoError(t, st.Close())
	st, err = store.Open(ctx, path)
	require.NoError(t, err)
	require.NoError(t, st.Close())
	fi, err := os.Stat(path)
	require.NoError(t, err)
	t.Logf("sqlite db size=%d", fi.Size())
}

// OAuth callback と同じ形（tcp4 127.0.0.1 の一時ポート）の待受に、同じプロセスから HTTP で届くか。
func TestProbeLoopbackCallback(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "code="+r.URL.Query().Get("code")) //nolint:gosec // 試作の検証用サーバー（自分のプロセスにしか返さない）
	}), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()
	resp, err := http.Get("http://" + ln.Addr().String() + "/callback?code=abc&state=s")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	require.Equal(t, "code=abc", string(b))
	t.Logf("loopback callback ok at %s", ln.Addr())
}

type nilBackend struct{ mcpserver.Backend }

// アプリ内 MCP サーバー: loopback で待ち受け、トークンなしは 401、あれば initialize が通る。
func TestProbeMCPServer(t *testing.T) {
	tokens := mcpserver.NewTokens(secrets.NewMemory())
	tok, err := tokens.Issue()
	require.NoError(t, err)
	m := mcpserver.New(mcpserver.Options{Backend: nilBackend{}, Tokens: tokens, Logger: slog.Default(), LogDir: t.TempDir(), Version: "probe"})
	require.NoError(t, m.Apply(mcpserver.Config{Enabled: true, Port: 47651}.Normalize()))
	defer m.Close()
	st := m.State()
	require.True(t, st.Listening, st.Err)
	url := "http://" + st.Addr + mcpserver.Path

	do := func(auth string) int {
		req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	require.Equal(t, http.StatusUnauthorized, do(""))
	require.Equal(t, http.StatusOK, do(tok))
	t.Logf("mcp initialize ok at %s", st.Addr)
}
