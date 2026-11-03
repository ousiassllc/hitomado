package mcpserver

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

func TestConfigDefaultsAreOffAndLoopback(t *testing.T) {
	c := Config{}.Normalize()
	assert.False(t, c.Enabled)
	assert.Equal(t, "127.0.0.1", c.Host)
	assert.Equal(t, DefaultPort, c.Port)
	require.NoError(t, c.Validate())
	assert.Equal(t, "http://127.0.0.1:47650/mcp", c.EndpointURL())
}

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"loopback v4", Config{Host: "127.0.0.1", Port: 8080}, true},
		{"loopback v6", Config{Host: "::1", Port: 8080}, true},
		{"localhost", Config{Host: "localhost", Port: 8080}, true},
		{"all interfaces without allow", Config{Host: "0.0.0.0", Port: 8080}, false},
		{"tailnet address without allow", Config{Host: "100.64.1.2", Port: 8080}, false},
		{"tailnet address with allow", Config{Host: "100.64.1.2", Port: 8080, AllowNonLoopback: true}, true},
		{"privileged port", Config{Host: "127.0.0.1", Port: 80}, false},
		{"port too large", Config{Host: "127.0.0.1", Port: 70000}, false},
		{"host with path", Config{Host: "127.0.0.1/x", Port: 8080}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestTokensVerify(t *testing.T) {
	store := secrets.NewMemory()
	tk := NewTokens(store)
	assert.False(t, tk.Has())
	assert.False(t, tk.Verify(""), "未発行で空は通らない")
	assert.False(t, tk.Verify("anything"))

	tok, err := tk.Issue()
	require.NoError(t, err)
	assert.Contains(t, tok, TokenPrefix)
	assert.True(t, tk.Has())
	assert.True(t, tk.Verify(tok))
	assert.False(t, tk.Verify(tok+"x"))
	assert.False(t, tk.Verify(""))

	// 再起動（別インスタンス）でも資格情報ストアの値で検証できる。
	assert.True(t, NewTokens(store).Verify(tok))

	tok2, err := tk.Issue()
	require.NoError(t, err)
	assert.NotEqual(t, tok, tok2)
	assert.False(t, tk.Verify(tok))

	require.NoError(t, tk.Revoke())
	assert.False(t, tk.Has())
	assert.False(t, tk.Verify(tok2))
	assert.False(t, NewTokens(store).Verify(tok2))
}

// Backend に送信に当たるメソッドを足せないよう、メソッド名を固定する（足すときはこのテストと docs/mcp.md を直す）。
func TestBackendHasNoSendMethods(t *testing.T) {
	typ := reflect.TypeFor[Backend]()
	var names []string
	for i := range typ.NumMethod() {
		names = append(names, typ.Method(i).Name)
	}
	assert.ElementsMatch(t, []string{"Status", "ListAccounts", "ListThreads", "SearchThreads", "GetThread", "CreateReplyDraft"}, names)
	for _, n := range names {
		assert.NotContains(t, strings.ToLower(n), "send")
		assert.NotContains(t, strings.ToLower(n), "confirm")
	}
}

type panicBackend struct{ Backend }

func (panicBackend) ListAccounts(context.Context) ([]AccountInfo, error) {
	panic("boom with refresh_token=EVADEP1 for alice@panic.example")
}

// ツールが panic しても接続は落とさず、MCP クライアントにはエラーを返し、ログにはスタックを残す（秘密は伏せる）。
func TestToolPanicIsRecoveredAndLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.NewWriter(&buf, slog.LevelDebug, logging.Meta{Version: "t"})
	srv := newServer(panicBackend{}, t.TempDir(), logger, "t")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(t.Context(), st, nil)
	require.NoError(t, err)
	defer func() { _ = ss.Close() }()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	defer func() { _ = cs.Close() }()

	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_accounts"})
	require.NoError(t, err)
	assert.True(t, res.IsError)
	_, err = cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_accounts"})
	require.NoError(t, err, "panic のあとも接続は使える")
	out := buf.String()
	assert.Contains(t, out, `"msg":"panic"`)
	assert.Contains(t, out, "mcp:list_accounts")
	assert.NotContains(t, out, "EVADEP1")
	assert.NotContains(t, out, "alice@panic.example")
}

func TestHostAllowed(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1:47650": true, "localhost:47650": true, "[::1]:47650": true, "100.64.1.2:1": true,
		"evil.example": false, "evil.example:47650": false, "mybox.tail.ts.net:47650": false,
	} {
		assert.Equal(t, want, hostAllowed(host, "127.0.0.1"), host)
	}
	assert.True(t, hostAllowed("mybox.tail.ts.net:47650", "mybox.tail.ts.net"), "設定したホスト名は通る")
}

func TestConfigNormalizeStripsBrackets(t *testing.T) {
	c := Config{Host: "[::1]", Port: 8080}.Normalize()
	assert.Equal(t, "::1", c.Host)
	assert.Equal(t, "[::1]:8080", c.Addr())
	require.NoError(t, c.Validate())
}

// 発行・検証・無効化を並行に行っても競合しない（go test -race で確かめる）。
func TestTokensConcurrentUse(t *testing.T) {
	tk := NewTokens(secrets.NewMemory())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				tok, err := tk.Issue()
				assert.NoError(t, err)
				tk.Verify(tok)
				tk.Has()
				if tok[len(tok)-1]%7 == 0 {
					assert.NoError(t, tk.Revoke())
				}
			}
		})
	}
	wg.Wait()
}

// Apply・Close・State を並行に呼んでも競合しない。
func TestManagerConcurrentApply(t *testing.T) {
	m := New(Options{Backend: panicBackend{}, Tokens: NewTokens(secrets.NewMemory()), Logger: slog.New(slog.DiscardHandler), LogDir: t.TempDir()})
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Go(func() {
			for range 5 {
				_ = m.Apply(Config{Enabled: i%2 == 0, Host: "127.0.0.1", Port: 0}.Normalize())
				m.State()
			}
		})
	}
	wg.Wait()
	m.Close()
	assert.False(t, m.State().Listening)
}

// MCP からは ChatGPT のログイン・トークン・AI 設定を操作できない（#75）。ツールと Backend のどちらにも、それらに当たる名前を足せない。
func TestMCPCannotTouchChatGPTLoginOrTokens(t *testing.T) {
	names := slices.Clone(ToolNames)
	typ := reflect.TypeFor[Backend]()
	for i := range typ.NumMethod() {
		names = append(names, typ.Method(i).Name)
	}
	for _, n := range names {
		l := strings.ToLower(n)
		for _, banned := range []string{"chatgpt", "openai", "signin", "sign_in", "login", "token", "oauth", "apikey", "api_key", "ai_", "aiconfig", "consent"} {
			assert.NotContains(t, l, banned, n)
		}
	}
}
