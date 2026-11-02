package mcptests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/logging"
)

func postWith(t *testing.T, endpoint, token string, mutate func(*http.Request)) int {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"0"}}}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	mutate(req)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// DNS rebinding・ブラウザからの横断リクエスト対策: 有効なトークンがあっても、ドメイン名の Host と別オリジンの Origin は断る。
func TestMCPRejectsForeignHostAndOrigin(t *testing.T) {
	m := newMCPEnv(t)
	st := m.enable(t)
	token := m.issueToken(t)

	assert.Equal(t, http.StatusOK, postWith(t, st.Endpoint, token, func(*http.Request) {}))
	assert.Equal(t, http.StatusForbidden, postWith(t, st.Endpoint, token, func(r *http.Request) { r.Host = "evil.example" }), "ドメイン名の Host")
	assert.Equal(t, http.StatusForbidden, postWith(t, st.Endpoint, token, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }), "別オリジン")
	assert.Equal(t, http.StatusForbidden, postWith(t, st.Endpoint, token, func(r *http.Request) { r.Header.Set("Origin", "null") }), "Origin: null")
	assert.Equal(t, http.StatusOK, postWith(t, st.Endpoint, token, func(r *http.Request) { r.Header.Set("Origin", "http://"+r.Host) }), "同一オリジンは通る")
	assert.Equal(t, http.StatusOK, postWith(t, st.Endpoint, token, func(r *http.Request) { r.Host = "localhost:" + strings.Split(r.Host, ":")[1] }))
}

// DB の設定が書き換えられても（許可なしの 0.0.0.0・範囲外のポート）、起動時に検証して待ち受けない。
func TestMCPStartRefusesInvalidSavedConfig(t *testing.T) {
	for name, raw := range map[string]string{
		"non-loopback without allow": `{"enabled":true,"host":"0.0.0.0","port":47661,"allowNonLoopback":false}`,
		"privileged port":            `{"enabled":true,"host":"127.0.0.1","port":80}`,
	} {
		t.Run(name, func(t *testing.T) {
			m := newMCPEnv(t)
			require.NoError(t, m.Store.SetSetting(t.Context(), "mcp.config", raw))
			m.Svc.Start(t.Context())
			st, err := m.Svc.GetMCPStatus(t.Context())
			require.NoError(t, err)
			assert.False(t, st.Listening)
			assert.NotEmpty(t, st.Error)
		})
	}
}

// 不正な値が残っていても、オフにはできる（保存済みの host・port は維持する）。
func TestMCPCanBeDisabledEvenWithInvalidInput(t *testing.T) {
	m := newMCPEnv(t)
	on := m.enable(t)
	st, err := m.Svc.SetMCPConfig(t.Context(), dto.MCPConfig{Enabled: false, Host: "0.0.0.0", Port: 80})
	require.NoError(t, err)
	assert.False(t, st.Enabled)
	assert.False(t, st.Listening)
	assert.Equal(t, on.Port, st.Port, "保存済みのポートを維持する")
	assert.Equal(t, on.Host, st.Host)
}

// 終了処理（Shutdown）のあとに設定が呼ばれても、再び待ち受けない。
func TestMCPDoesNotListenAfterShutdown(t *testing.T) {
	m := newMCPEnv(t)
	m.enable(t)
	m.Svc.Shutdown()
	st, err := m.Svc.SetMCPConfig(t.Context(), dto.MCPConfig{Enabled: true, Host: "127.0.0.1", Port: freePort(t)})
	require.NoError(t, err)
	assert.False(t, st.Listening)
}

// トークンの再発行で待ち受けを作り直し、古いトークンで確立済みの接続（ストリーム）は閉じる。新しいトークンでは引き続き使える。
func TestMCPTokenRotationClosesEstablishedConnections(t *testing.T) {
	m := newMCPEnv(t)
	st := m.enable(t)
	old := m.issueToken(t)
	c := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	oldSession, err := c.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint: st.Endpoint, HTTPClient: &http.Client{Transport: bearerTransport{old}}, MaxRetries: -1,
	}, nil) // 常設の SSE ストリームを開く
	require.NoError(t, err)
	t.Cleanup(func() { _ = oldSession.Close() })
	_, err = oldSession.ListTools(t.Context(), nil)
	require.NoError(t, err)

	fresh := m.issueToken(t)

	closed := make(chan struct{})
	go func() { _ = oldSession.Wait(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("再発行のあとも、古いトークンの接続が閉じない")
	}
	cur, err := m.Svc.GetMCPStatus(t.Context())
	require.NoError(t, err)
	assert.True(t, cur.Listening)
	_, err = connect(t, st.Endpoint, fresh).ListTools(t.Context(), nil)
	require.NoError(t, err)
}

// ツール本体まで届かない呼び出し（未知のツール名・不正な引数）も記録する。
func TestMCPProtocolErrorsAreLogged(t *testing.T) {
	m := newMCPEnv(t)
	st := m.enable(t)
	cs := connect(t, st.Endpoint, m.issueToken(t))
	_, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "send_message", Arguments: map[string]any{}})
	require.Error(t, err)
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_thread", Arguments: map[string]any{"accountId": 123}})
	require.NoError(t, err)
	require.True(t, res.IsError, "スキーマに合わない引数はエラー結果")

	entries, err := logging.ReadEntries(m.LogDir, logging.Query{})
	require.NoError(t, err)
	var tools []string
	for _, e := range entries {
		if e.Message == "mcp tool call" && e.Attrs["error_code"] == "PROTOCOL" {
			tool, _ := e.Attrs["tool"].(string)
			tools = append(tools, tool)
			assert.Equal(t, false, e.Attrs["ok"])
		}
	}
	assert.Equal(t, []string{"send_message", "get_thread"}, tools)
}

// 同じスレッドに下書きがあるときは新しく作らず、既存の下書きの ID を知らせる。
func TestMCPReplyDraftDoesNotDuplicate(t *testing.T) {
	m := newMCPEnv(t)
	a := m.AddAccount(t, "me@a.example")
	msg := apptest.Msg("m1", "t1", at(1), "INBOX")
	msg.From = model.Address{Name: "Alice", Email: "alice@x.example"}
	m.PutThread(t, a.ID, "t1", msg)
	st := m.enable(t)
	cs := connect(t, st.Endpoint, m.issueToken(t))

	var first struct {
		DraftID string `json:"draftId"`
	}
	structured(t, call(t, cs, "create_reply_draft", map[string]any{"accountId": a.ID, "threadId": "t1", "body": "一回目"}), &first)
	res := call(t, cs, "create_reply_draft", map[string]any{"accountId": a.ID, "threadId": "t1", "body": "二回目"})
	require.True(t, res.IsError)
	b, _ := json.Marshal(res.Content)
	assert.Contains(t, string(b), first.DraftID, "既存の下書き ID を返す")
	assert.Equal(t, 1, m.Gmail.CallCount("CreateDraft"))
	assert.Len(t, m.Gmail.Drafts, 1)
}

// Host／Origin の拒否も（接続元だけ）ログに残す。
func TestMCPForeignHostRejectionIsLogged(t *testing.T) {
	m := newMCPEnv(t)
	st := m.enable(t)
	token := m.issueToken(t)
	assert.Equal(t, http.StatusForbidden, postWith(t, st.Endpoint, token, func(r *http.Request) { r.Host = "evil.example" }))
	log := m.logText(t)
	assert.Contains(t, log, "mcp connection rejected")
	assert.Contains(t, log, `"status":403`)
	assert.NotContains(t, log, token)
}
