package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strconv"
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

// キー・値の形をずらした回避形（区切り・大文字小文字・前置・URL エンコード・JSON のキー名）も伏せる。
func TestRedactCatchesEvasiveForms(t *testing.T) {
	cases := map[string]string{ //nolint:gosec // 伏せ処理の検証用の見本（実在しない値）
		"underscore prefix":   "session_token=EVADE1 mcp_token=EVADE2 x_client_secret=EVADE3",
		"mixed case":          "Session_Token=EVADE4 ACCESS_TOKEN=EVADE5",
		"header colon":        "X-Auth-Token: EVADE6",
		"token colon":         "token: EVADE7",
		"basic authorization": "Authorization: Basic dXNlcjpFVkFERTg=",
		"bearer encoded":      "Authorization=Bearer%20EVADE9",
		"url encoded pair":    "https://x/cb?access_token%3DEVADE10&ok=1",
		"json camel":          `{"accessToken":"EVADE11","refreshToken":"EVADE12","token":"EVADE13","secret":"EVADE14"}`,
		"query code":          "GET /cb?code=EVADE15&state=EVADE16",
		"google api key":      "https://x/?key=AIzaSyEVADE17EVADE17EVADE17EVADE17EVA",
		"access key":          "access_key=EVADE18 secret_key=EVADE19",
		"unicode email":       "failed for 山田@example.com",
		"encoded email":       "to=alice%40evade20.example",
	}
	for name, in := range cases {
		out := logOnce(t, func(l *slog.Logger) { l.Error("x", "detail", in) })
		for i := 1; i <= 20; i++ {
			assert.NotContains(t, out, "EVADE"+itoa(i)+"\"", name)
			assert.NotRegexp(t, "EVADE"+itoa(i)+`\b`, out, name)
		}
		assert.NotContains(t, out, "山田@example.com", name)
		assert.NotContains(t, out, "alice@evade20", name)
		assert.NotContains(t, out, "alice%40evade20", name)
		assert.NotContains(t, out, "dXNlcjpFVkFERTg", name)
		assert.NotContains(t, out, "AIzaSy", name)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

// 秘密・メール内容の名前を持つキー（回避しやすい別名を含む）。
func TestRedactMasksMoreKeyNames(t *testing.T) {
	out := logOnce(t, func(l *slog.Logger) {
		l.Info("x",
			"emails", "EVADEK1", "fromAddress", "EVADEK2", "senderEmail", "EVADEK3", "htmlContent", "EVADEK4",
			"quoteHtml", "EVADEK5", "signatureHtml", "EVADEK6", "q", "EVADEK7", "query", "EVADEK8", "preview", "EVADEK9",
			"displayName", "EVADEK10", "pwd", "EVADEK11", "passphrase", "EVADEK12", "jwt", "EVADEK13", "sessionId", "EVADEK14",
			"otp", "EVADEK15", "codeVerifier", "EVADEK16",
		)
	})
	assert.NotContains(t, out, "EVADEK")
}

// 名前が秘密・メール内容のグループ（Group・WithGroup）は中身を丸ごと伏せる。
func TestRedactMasksSensitiveGroupNames(t *testing.T) {
	out := logOnce(t, func(l *slog.Logger) {
		l.Info("x", slog.Group("subject", "x", "EVADEG1"), slog.Group("credentials", "value", "EVADEG2", slog.Group("inner", "v", "EVADEG3")))
		l.WithGroup("password").Info("y", "value", "EVADEG4")
		l.WithGroup("req").With("a", "keep-a").WithGroup("body").Info("z", "n", "EVADEG5")
	})
	assert.NotContains(t, out, "EVADEG")
	assert.Contains(t, out, "keep-a")
}

// time という名前の文字列属性が ReplaceAttr で panic しない。Stringer の panic もロガーの外へ出さない。
type panicStringer struct{}

func (*panicStringer) String() string { panic("stringer boom") }

func TestRedactSurvivesHostileValues(t *testing.T) {
	var nilStringer *panicStringer
	out := logOnce(t, func(l *slog.Logger) {
		l.Info("x", "time", "not a time", "v", nilStringer)
	})
	assert.Contains(t, out, `"time":"not a time"`)
	assert.Contains(t, out, `<*logging.panicStringer>`)
}

func TestScrubDoesNotOvermaskStatusCodes(t *testing.T) {
	assert.Equal(t, "gmail: status code=404 for thread", Scrub("gmail: status code=404 for thread"))
	assert.Equal(t, "a 100%% done", Scrub("a 100%% done"))
}

// スタックトレースの「ファイル名.go:行」は、名前に token・session などを含んでいても伏せない（調査に要る）。
func TestScrubKeepsStackTraceFileLines(t *testing.T) {
	for _, in := range []string{
		"/app/internal/app/composecore/compose_session.go:127 +0x1d",
		"internal/oauth/token.go:67",
		"frontend/src/session.ts:12:5",
	} {
		assert.Equal(t, in, Scrub(in))
	}
	for _, in := range []string{
		"access_token=EVADEF1", "x.session_token=EVADEF2", "token.value=EVADEF3", "client.secret.value=EVADEF4",
		"tokens.value=EVADEF5", "token=EVADEF6%2BEVADEF7", "token%3DEVADEF8%2BEVADEF9", "X-Auth-Token: EVADEFA",
	} {
		assert.NotContains(t, Scrub(in), "EVADEF", in)
	}
}
