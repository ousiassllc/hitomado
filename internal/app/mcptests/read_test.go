package mcptests

import (
	"bytes"
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/mcpserver"
)

func TestMCPReadTools(t *testing.T) {
	m := newMCPEnv(t)
	a := m.AddAccount(t, "me@a.example")
	b := m.AddAccount(t, "me@b.example")
	msg := apptest.Msg("m1", "t1", at(1), "INBOX", "UNREAD")
	msg.BodyHTML = "<p>Hello <b>world</b></p>"
	msg.Attachments = []model.AttachmentInfo{{ID: "att1", Filename: "report.pdf", MimeType: "application/pdf", Size: 1234}}
	m.PutThread(t, a.ID, "t1", msg)
	m.PutThread(t, b.ID, "t2", apptest.Msg("m2", "t2", at(2), "INBOX"))
	exhaust(t, m.Env, a.ID, b.ID)
	st := m.enable(t)
	cs := connect(t, st.Endpoint, m.issueToken(t))

	var status struct {
		Version  string `json:"version"`
		Accounts []struct {
			AccountID       string `json:"accountId"`
			Email           string `json:"email"`
			InitialSyncDone bool   `json:"initialSyncDone"`
		} `json:"accounts"`
	}
	structured(t, call(t, cs, "get_status", nil), &status)
	assert.Equal(t, "0.1.0", status.Version)
	require.Len(t, status.Accounts, 2)
	assert.True(t, status.Accounts[0].InitialSyncDone)

	var accounts struct {
		Accounts []mcpserver.AccountInfo `json:"accounts"`
	}
	structured(t, call(t, cs, "list_accounts", nil), &accounts)
	require.Len(t, accounts.Accounts, 2)
	assert.ElementsMatch(t, []string{"me@a.example", "me@b.example"}, []string{accounts.Accounts[0].Email, accounts.Accounts[1].Email})

	var page dto.ThreadPage
	structured(t, call(t, cs, "list_threads", map[string]any{"limit": 10}), &page)
	assert.Equal(t, []string{"t2", "t1"}, threadIDs(page.Items), "新しい順")
	structured(t, call(t, cs, "list_threads", map[string]any{"accountIds": []string{a.ID}, "unreadOnly": true}), &page)
	assert.Equal(t, []string{"t1"}, threadIDs(page.Items))

	m.Gmail.Lists[apptest.Key(a.ID, "from:sender")] = []gmail.ThreadStub{{ID: "t1"}}
	structured(t, call(t, cs, "search_threads", map[string]any{"query": "from:sender", "accountIds": []string{a.ID}}), &page)
	assert.Equal(t, []string{"t1"}, threadIDs(page.Items))

	var thread mcpserver.ThreadView
	structured(t, call(t, cs, "get_thread", map[string]any{"accountId": a.ID, "threadId": "t1"}), &thread)
	require.Len(t, thread.Messages, 1)
	assert.Equal(t, "subject t1", thread.Subject)
	assert.Contains(t, thread.Messages[0].BodyText, "Hello world", "HTML はプレーンテキストにして返す")
	assert.NotContains(t, thread.Messages[0].BodyText, "<b>")
	assert.True(t, thread.Messages[0].IsUnread)
	assert.Equal(t, []model.AttachmentInfo{{ID: "att1", Filename: "report.pdf", MimeType: "application/pdf", Size: 1234}}, thread.Messages[0].Attachments,
		"添付は名前・種類・サイズだけ")
	assert.Zero(t, m.Gmail.CallCount("GetAttachment"), "添付の中身は取りに行かない")
	assert.Empty(t, m.Gmail.Modified, "読み取りでは既読にしない")

	res := call(t, cs, "get_thread", map[string]any{"accountId": a.ID, "threadId": "nope"})
	assert.True(t, res.IsError)
}

func TestMCPGetLogsFiltersByLevelAndLimit(t *testing.T) {
	m := newMCPEnv(t)
	st := m.enable(t)
	cs := connect(t, st.Endpoint, m.issueToken(t))
	m.Deps.Logger.Info("info line")
	m.Deps.Logger.Warn("warn line")
	m.Deps.Logger.Error("error line one", "account_id", "01HX")
	m.Deps.Logger.Error("error line two")

	type out struct {
		Entries []struct {
			Level   string         `json:"level"`
			Message string         `json:"message"`
			Attrs   map[string]any `json:"attrs"`
		} `json:"entries"`
	}
	var got out
	structured(t, call(t, cs, "get_logs", map[string]any{"level": "error"}), &got)
	require.Len(t, got.Entries, 2)
	assert.Equal(t, "error line one", got.Entries[0].Message)
	assert.Equal(t, "01HX", got.Entries[0].Attrs["account_id"])
	assert.Equal(t, "t", got.Entries[0].Attrs["app_version"])

	structured(t, call(t, cs, "get_logs", map[string]any{"level": "error", "limit": 1}), &got)
	require.Len(t, got.Entries, 1)
	assert.Equal(t, "error line two", got.Entries[0].Message, "新しい方から")

	structured(t, call(t, cs, "get_logs", map[string]any{"since": "2999-01-01T00:00:00Z"}), &got)
	assert.Empty(t, got.Entries)

	assert.True(t, call(t, cs, "get_logs", map[string]any{"level": "loud"}).IsError)
	assert.True(t, call(t, cs, "get_logs", map[string]any{"since": "yesterday"}).IsError)

	var status struct {
		LastLogError *struct {
			Message string `json:"message"`
		} `json:"lastLogError"`
	}
	structured(t, call(t, cs, "get_status", nil), &status)
	require.NotNil(t, status.LastLogError)
	assert.Equal(t, "error line two", status.LastLogError.Message, "状態に最後のエラーが出る")
}

// --- 仕様 (3): MCP の呼び出しはログに残るが、本文・検索語・トークンは残らない ---

func TestMCPCallsAreLoggedWithoutBodiesQueriesOrToken(t *testing.T) {
	m := newMCPEnv(t)
	a := m.AddAccount(t, "me@a.example")
	msg := apptest.Msg("m1", "t1", at(1), "INBOX")
	msg.From = model.Address{Name: "Alice", Email: "alice.private@example.com"}
	msg.Subject = "極秘の見積もり"
	msg.BodyHTML = "<p>本文：来週の価格は 1,234 円です</p>"
	m.PutThread(t, a.ID, "t1", msg)
	m.Gmail.Lists[apptest.Key(a.ID, "confidential-term")] = []gmail.ThreadStub{{ID: "t1"}}
	st := m.enable(t)
	token := m.issueToken(t)
	cs := connect(t, st.Endpoint, token)

	call(t, cs, "get_thread", map[string]any{"accountId": a.ID, "threadId": "t1"})
	call(t, cs, "search_threads", map[string]any{"query": "confidential-term"})
	call(t, cs, "create_reply_draft", map[string]any{"accountId": a.ID, "threadId": "t1", "body": "返信の秘密の本文 SECRET-REPLY-BODY"})
	call(t, cs, "create_reply_draft", map[string]any{"accountId": a.ID, "threadId": "t1", "body": ""}) // 失敗

	log := m.logText(t)
	entries, err := logging.ReadEntries(m.LogDir, logging.Query{})
	require.NoError(t, err)
	type callRec struct {
		ok        any
		errorCode any
	}
	calls := map[string][]callRec{}
	for _, e := range entries {
		if e.Message == "mcp tool call" {
			tool, _ := e.Attrs["tool"].(string)
			calls[tool] = append(calls[tool], callRec{e.Attrs["ok"], e.Attrs["error_code"]})
		}
	}
	assert.Equal(t, []callRec{{true, nil}}, calls["get_thread"], "いつ・どのツール・成否が残る")
	assert.Len(t, calls["search_threads"], 1)
	require.Len(t, calls["create_reply_draft"], 2)
	assert.Equal(t, callRec{true, nil}, calls["create_reply_draft"][0])
	assert.Equal(t, callRec{false, "VALIDATION"}, calls["create_reply_draft"][1], "失敗はエラーコードだけ")

	for _, secret := range []string{
		token, "alice.private@example.com", "極秘の見積もり", "来週の価格", "confidential-term", "SECRET-REPLY-BODY", "返信の秘密の本文",
	} {
		assert.NotContains(t, log, secret)
	}
}

func TestMCPToolErrorsDoNotLeakInternalDetails(t *testing.T) {
	m := newMCPEnv(t)
	a := m.AddAccount(t, "me@a.example")
	m.Gmail.Err = map[string]error{"GetThreadFull": context.DeadlineExceeded}
	st := m.enable(t)
	cs := connect(t, st.Endpoint, m.issueToken(t))
	res := call(t, cs, "get_thread", map[string]any{"accountId": a.ID, "threadId": "t1"})
	require.True(t, res.IsError)
	var sb bytes.Buffer
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	assert.NotContains(t, sb.String(), "deadline exceeded")
}
