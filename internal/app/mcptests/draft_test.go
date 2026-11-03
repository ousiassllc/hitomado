package mcptests

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/mcpserver"
)

func TestMCPHasNoSendTool(t *testing.T) {
	m := newMCPEnv(t)
	st := m.enable(t)
	cs := connect(t, st.Endpoint, m.issueToken(t))

	var names []string
	for tool, err := range cs.Tools(t.Context(), nil) {
		require.NoError(t, err)
		names = append(names, tool.Name)
		if tool.Name == "create_reply_draft" {
			require.NotNil(t, tool.Annotations)
			assert.False(t, tool.Annotations.ReadOnlyHint, "下書きを作るツールは読み取り専用ではない")
		} else {
			require.NotNil(t, tool.Annotations)
			assert.True(t, tool.Annotations.ReadOnlyHint, "%s は読み取り専用", tool.Name)
		}
	}
	assert.ElementsMatch(t, mcpserver.ToolNames, names, "公開するツールは ToolNames だけ")
	for _, n := range names {
		assert.NotContains(t, strings.ToLower(n), "send", "送信するツールは作らない: %s", n)
	}

	for _, name := range []string{"send_message", "send_email", "send_draft", "send_reply", "prepare_send", "forward_message"} {
		_, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{"accountId": "x"}})
		assert.Error(t, err, "%s は呼べない（未知のツール）", name)
	}
}

func TestMCPCreateReplyDraftNeverSends(t *testing.T) {
	m := newMCPEnv(t)
	a := m.AddAccount(t, "me@a.example")
	msg := apptest.Msg("m1", "t1", at(1), "INBOX")
	msg.From = model.Address{Name: "Alice", Email: "alice@x.example"}
	m.PutThread(t, a.ID, "t1", msg)
	st := m.enable(t)
	cs := connect(t, st.Endpoint, m.issueToken(t))

	res := call(t, cs, "create_reply_draft", map[string]any{"accountId": a.ID, "threadId": "t1", "body": "了解しました。"})
	var out mcpserver.ReplyDraft
	structured(t, res, &out)
	assert.NotEmpty(t, out.DraftID)

	assert.Empty(t, m.Gmail.Sent, "送信されていない")
	assert.Zero(t, m.Gmail.CallCount("Send"))
	assert.Equal(t, 1, m.Gmail.CallCount("CreateDraft"))
}

// --- 仕様 (3 の一部): 下書きの送信元は受信アカウントに固定 ---

func TestMCPReplyDraftSenderIsPinnedToReceivingAccount(t *testing.T) {
	m := newMCPEnv(t)
	a := m.AddAccount(t, "me@a.example")
	b := m.AddAccount(t, "me@b.example")
	_, err := m.Store.SetSignature(t.Context(), b.ID, "<p>Sig B</p>")
	require.NoError(t, err)

	// 同じ相手からのスレッドが A と B の両方にある。B 側のスレッドへ返信する。
	for _, acc := range []string{a.ID, b.ID} {
		msg := apptest.Msg("m-"+acc, "t1", at(1), "INBOX")
		msg.From = model.Address{Name: "Alice", Email: "alice@x.example"}
		msg.Subject = "Hello"
		msg.MessageID = "<orig-" + acc + "@x.example>"
		m.PutThread(t, acc, "t1", msg)
	}
	st := m.enable(t)
	cs := connect(t, st.Endpoint, m.issueToken(t))

	var out mcpserver.ReplyDraft
	structured(t, call(t, cs, "create_reply_draft", map[string]any{"accountId": b.ID, "threadId": "t1", "body": "返信本文です"}), &out)

	assert.Equal(t, b.ID, out.AccountID)
	assert.Equal(t, "me@b.example", out.From)
	assert.Equal(t, "Re: Hello", out.Subject)
	require.Len(t, out.To, 1)
	assert.Equal(t, "alice@x.example", out.To[0].Email)

	raw := string(m.Gmail.DraftRaw[apptest.Key(b.ID, out.DraftID)])
	require.NotEmpty(t, raw, "下書きは B のアカウントに作られる")
	assert.Contains(t, raw, "me@b.example")
	assert.NotContains(t, raw, "me@a.example")
	assert.Contains(t, raw, "In-Reply-To: <orig-"+b.ID+"@x.example>")
	assert.Empty(t, m.Gmail.DraftRaw[apptest.Key(a.ID, out.DraftID)], "A には作られない")
	assert.Equal(t, apptest.Key(b.ID, out.DraftID), onlyKey(t, m.Gmail.Drafts))
	assert.Equal(t, "t1", m.Gmail.Drafts[apptest.Key(b.ID, out.DraftID)].ThreadID)

	d := m.Gmail.Drafts[apptest.Key(b.ID, out.DraftID)]
	assert.Contains(t, d.Message.BodyHTML, "返信本文です")
	assert.Contains(t, d.Message.BodyHTML, "Sig B", "会社ごとの署名は今のルールどおり付く")
	assert.Less(t, strings.Index(d.Message.BodyHTML, "返信本文です"), strings.Index(d.Message.BodyHTML, "Sig B"), "本文は署名の前")
}

func onlyKey(t *testing.T, m map[string]gmail.DraftFull) string {
	t.Helper()
	require.Len(t, m, 1)
	for k := range m {
		return k
	}
	return ""
}

func TestMCPReplyDraftValidatesInput(t *testing.T) {
	m := newMCPEnv(t)
	a := m.AddAccount(t, "me@a.example")
	m.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", at(1), "INBOX"))
	st := m.enable(t)
	cs := connect(t, st.Endpoint, m.issueToken(t))

	res := call(t, cs, "create_reply_draft", map[string]any{"accountId": a.ID, "threadId": "t1", "body": "  "})
	assert.True(t, res.IsError, "本文が空は拒否")
	res = call(t, cs, "create_reply_draft", map[string]any{"accountId": "ghost", "threadId": "t1", "body": "x"})
	assert.True(t, res.IsError, "存在しないアカウント")
	assert.Empty(t, m.Gmail.Drafts)
}

// --- ツール: 読み取り ---
