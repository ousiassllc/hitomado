package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 実在しない見本の秘密・メール内容。出力のどこにも現れてはならない。
var (
	fakeSecrets = []string{
		"ya29.FAKEACCESSTOKENVALUE",
		"1//FAKEREFRESHTOKENVALUE0123456789",
		"GOCSPX-FAKECLIENTSECRET",
		"sk-ant-FAKEAPIKEYVALUE0123456789",
		"eyJhbGciOiJub25lIn0.eyJzdWIiOiJmYWtlIn0.FAKESIG",
		"hmcp_FAKEMCPTOKENVALUE",
		"hunter2-fake-password",
	}
	fakeMailContent = []string{
		"件名：極秘の見積もり",
		"本文：来週の価格は 1,234 円です",
		"alice.private@example.com",
		"bob.private@example.org",
		"secret-report.pdf",
	}
)

func logOnce(t *testing.T, fn func(l *slog.Logger)) string {
	t.Helper()
	var buf bytes.Buffer
	fn(NewWriter(&buf, slog.LevelDebug, Meta{Version: "t"}))
	return buf.String()
}

func requireNoLeak(t *testing.T, out string) {
	t.Helper()
	for _, s := range append(append([]string{}, fakeSecrets...), fakeMailContent...) {
		assert.NotContains(t, out, s)
	}
}

// 秘密のキー・メール内容のキーは値を丸ごと伏せる（値の形に依存しない）。
func TestRedactMasksSensitiveKeys(t *testing.T) {
	out := logOnce(t, func(l *slog.Logger) {
		l.Info("event",
			"access_token", fakeSecrets[0], "refreshToken", fakeSecrets[1], "client_secret", fakeSecrets[2],
			"apiKey", fakeSecrets[3], "Authorization", "Bearer "+fakeSecrets[5], "password", fakeSecrets[6],
			"mcpToken", "plain-without-prefix",
			"subject", fakeMailContent[0], "bodyHtml", fakeMailContent[1], "body", fakeMailContent[1],
			"snippet", fakeMailContent[1], "to", fakeMailContent[2], "cc", fakeMailContent[3], "from", fakeMailContent[2],
			"attachmentName", fakeMailContent[4], "filename", fakeMailContent[4],
			"accountId", "01HXKEEP",
		)
	})
	requireNoLeak(t, out)
	assert.NotContains(t, out, "plain-without-prefix")
	assert.Contains(t, out, `"accountId":"01HXKEEP"`, "アカウント ID は残す")
	assert.Contains(t, out, Masked)
}

// 自由な文字列（メッセージ・エラー文・URL）に混ざった秘密とアドレスも伏せる。
func TestRedactScrubsSecretsInsideStrings(t *testing.T) {
	out := logOnce(t, func(l *slog.Logger) {
		l.Error("oauth failed for "+fakeMailContent[2]+" with "+fakeSecrets[0],
			"err", errors.New("Post https://oauth2.example/token?code=abc&refresh_token="+fakeSecrets[1]+": denied "+fakeMailContent[3]),
			"detail", `{"access_token":"`+fakeSecrets[0]+`","client_secret":"`+fakeSecrets[2]+`"}`,
			"header", "Authorization: Bearer "+fakeSecrets[4],
			"key", fakeSecrets[3],
			"note", "token "+fakeSecrets[5],
		)
	})
	requireNoLeak(t, out)
	assert.Contains(t, out, "oauth failed")
	assert.Contains(t, out, "[email]")
}

// 構造体・マップ・グループの中でも伏せる。中身の分からない値は型名だけにする。
func TestRedactHandlesGroupsWithAttrsAndOpaqueValues(t *testing.T) {
	type mail struct{ Subject, Body string }
	out := logOnce(t, func(l *slog.Logger) {
		l = l.With("session_token", fakeSecrets[0]).WithGroup("req")
		l.Warn("rejected",
			slog.Group("headers", slog.String("cookie", fakeSecrets[6]), slog.String("method", "POST")),
			"payload", mail{Subject: fakeMailContent[0], Body: fakeMailContent[1]},
			"rows", map[string]string{"to": fakeMailContent[2]},
		)
	})
	requireNoLeak(t, out)
	assert.Contains(t, out, `"method":"POST"`, "秘密でない値は残す")
}

// 件数・時間のような数値は変えない。
func TestRedactKeepsPlainValues(t *testing.T) {
	out := logOnce(t, func(l *slog.Logger) {
		l.Info("sync done", "added", 3, "ok", true, "account_id", "01HX")
	})
	assert.Contains(t, out, `"added":3`)
	assert.Contains(t, out, `"ok":true`)
	assert.NotContains(t, out, Masked)
}

func TestScrubKeepsOrdinaryText(t *testing.T) {
	for _, s := range []string{"sync done", "rate limited, retry in 5s", "gmail: 404 not found", "code 404"} {
		require.Equal(t, s, Scrub(s))
	}
}
