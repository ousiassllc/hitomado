package mcpserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
