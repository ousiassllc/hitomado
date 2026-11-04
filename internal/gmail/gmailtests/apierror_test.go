package gmailtests

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/gmail/googleerr"
)

// Google のエラー message に混ざり得るトークン風のダミー値。伏せ字処理で消えること。
const leakedToken = "ya29.DUMMY-access-token_0123456789abcdef" //nolint:gosec // 漏えい検査用のダミー値

// lockedBuffer はログの出力先（複数 goroutine から書かれても壊れないようにする）。
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// logRecords は JSON ログの各行を読む。1 件が 1 行に収まっていること（改行によるログ注入がないこと）も確かめる。
func (l *lockedBuffer) records(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(l.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), "ログの 1 行が JSON として読めない: %q", line)
		out = append(out, m)
	}
	return out
}

// newLoggedEnv は伏せ字処理をかけない素の slog ハンドラにログを出すテスト環境。
// 本番の Redact に頼らず、gmail パッケージ自身が値を伏せていることを確かめる。
func newLoggedEnv(t *testing.T) (*testEnv, *lockedBuffer) {
	t.Helper()
	buf := &lockedBuffer{}
	e := newTestEnvWith(t, func(o *gmail.Options) {
		o.Logger = slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})
	return e, buf
}

// googleError は Google の標準的なエラー応答（error.status・errors[]・details[]）。
func googleError(code int, status, message string, errs []string, details []map[string]any) map[string]any {
	items := make([]map[string]any, 0, len(errs))
	for _, r := range errs {
		items = append(items, map[string]any{"reason": r, "message": message})
	}
	body := map[string]any{"code": code, "message": message, "status": status, "errors": items}
	if details != nil {
		body["details"] = details
	}
	return map[string]any{"error": body}
}

func errorInfo(reason string, metadata map[string]any) map[string]any {
	return map[string]any{
		"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": reason, "domain": "googleapis.com", "metadata": metadata,
	}
}

func TestAPIDisabledReasonIsInErrorAndLog(t *testing.T) {
	e, logs := newLoggedEnv(t)
	msg := "Gmail API has not been used in project 123456789 before or it is disabled. access_token=" + leakedToken
	e.f.HandleJSON("GET "+apiBase+"/profile", 403, googleError(403, "PERMISSION_DENIED", msg,
		[]string{"accessNotConfigured"},
		[]map[string]any{errorInfo("SERVICE_DISABLED", map[string]any{
			"service":       "gmail.googleapis.com",
			"consumer":      "projects/123456789",
			"activationUrl": "https://console.developers.google.com/apis/api/gmail.googleapis.com/overview?project=123456789",
			"internalNote":  "not-on-the-allowlist-" + leakedToken,
		})}))

	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, gmail.ErrGeneric)
	require.ErrorIs(t, err, gmail.ErrAPIDisabled)
	assert.NotErrorIs(t, err, gmail.ErrScopeInsufficient)
	assert.Len(t, e.f.Requests("GET", apiBase+"/profile"), 1, "再試行しない")
	for _, want := range []string{"HTTP 403", "PERMISSION_DENIED", "accessNotConfigured", "SERVICE_DISABLED", "Gmail API has not been used"} {
		assert.Contains(t, err.Error(), want)
	}
	assert.NotContains(t, err.Error(), leakedToken)

	ae, ok := gmail.AsAPIError(err)
	require.True(t, ok)
	assert.Equal(t, "getProfile", ae.Op)
	assert.Equal(t, 403, ae.Status)
	assert.Equal(t, []string{"accessNotConfigured", "SERVICE_DISABLED"}, ae.Reasons)
	assert.Equal(t, "gmail.googleapis.com", ae.Metadata["service"])
	assert.NotContains(t, ae.Metadata, "internalNote")

	recs := logs.records(t)
	require.Len(t, recs, 1)
	assert.Equal(t, "WARN", recs[0]["level"])
	assert.Equal(t, "getProfile", recs[0]["op"])
	assert.EqualValues(t, 403, recs[0]["status"])
	assert.Equal(t, "PERMISSION_DENIED", recs[0]["api_status"])
	assert.Equal(t, "accessNotConfigured,SERVICE_DISABLED", recs[0]["reasons"])
	assert.Contains(t, recs[0]["api_message"], "Gmail API has not been used")
	assert.Contains(t, recs[0]["api_metadata"], "service=gmail.googleapis.com")
	assert.NotContains(t, logs.String(), leakedToken, "ログにトークン風の値が出ない")
	assert.NotContains(t, logs.String(), "not-on-the-allowlist")
	assert.Contains(t, logs.String(), "[REDACTED]")
}

func TestScopeInsufficientReasonIsInErrorAndLog(t *testing.T) {
	e, logs := newLoggedEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 403, googleError(403, "PERMISSION_DENIED",
		"Request had insufficient authentication scopes. Authorization: Bearer "+leakedToken,
		[]string{"insufficientPermissions"},
		[]map[string]any{errorInfo("ACCESS_TOKEN_SCOPE_INSUFFICIENT", map[string]any{"method": "google.gmail.v1.UsersService.GetProfile", "service": "gmail.googleapis.com"})}))

	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, gmail.ErrScopeInsufficient)
	require.ErrorIs(t, err, gmail.ErrNeedsReauth, "再接続して許可し直す必要があるので要再接続に分類する")
	assert.Len(t, e.f.Requests("GET", apiBase+"/profile"), 1, "再試行しない")
	assert.Contains(t, err.Error(), "HTTP 403")
	for _, want := range []string{"insufficientPermissions", "ACCESS_TOKEN_SCOPE_INSUFFICIENT"} {
		assert.Contains(t, err.Error(), want)
		assert.Contains(t, logs.String(), want)
	}
	assert.NotContains(t, err.Error(), leakedToken)
	assert.NotContains(t, logs.String(), leakedToken)
}

func TestUnauthorizedReasonIsInErrorAndLog(t *testing.T) {
	e, logs := newLoggedEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 401, googleError(401, "UNAUTHENTICATED",
		"Invalid Credentials "+leakedToken,
		[]string{"authError"},
		[]map[string]any{errorInfo("ACCESS_TOKEN_EXPIRED", map[string]any{"service": "gmail.googleapis.com"})}))

	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, gmail.ErrNeedsReauth)
	assert.NotErrorIs(t, err, gmail.ErrScopeInsufficient)
	assert.Equal(t, 2, e.f.TokenRequests(), "401 のあとアクセストークンを取り直して 1 回だけやり直す")
	assert.Contains(t, err.Error(), "HTTP 401")
	for _, want := range []string{"UNAUTHENTICATED", "authError", "ACCESS_TOKEN_EXPIRED"} {
		assert.Contains(t, err.Error(), want)
		assert.Contains(t, logs.String(), want)
	}
	assert.NotContains(t, err.Error(), leakedToken)
	assert.NotContains(t, logs.String(), leakedToken)
}

// getProfile 以外の呼び出し（共通のエラー処理を通るすべての API）でも reason が載る。
func TestReasonIsIncludedForOtherCalls(t *testing.T) {
	e, logs := newLoggedEnv(t)
	body := googleError(403, "PERMISSION_DENIED", "disabled", []string{"accessNotConfigured"},
		[]map[string]any{errorInfo("SERVICE_DISABLED", map[string]any{"service": "gmail.googleapis.com"})})
	e.f.HandleJSON("GET "+apiBase+"/threads", 403, body)
	e.f.HandleJSON("POST "+apiBase+"/threads/t1/modify", 403, body)

	_, err := e.c.ListThreads(context.Background(), testAccount, gmail.ListQuery{})
	require.ErrorIs(t, err, gmail.ErrAPIDisabled)
	assert.Contains(t, err.Error(), "SERVICE_DISABLED")
	err = e.c.ModifyThread(context.Background(), testAccount, "t1", nil, nil)
	require.ErrorIs(t, err, gmail.ErrAPIDisabled)
	assert.Contains(t, err.Error(), "SERVICE_DISABLED")

	recs := logs.records(t)
	require.Len(t, recs, 2)
	assert.Equal(t, "threads.list", recs[0]["op"])
	assert.Equal(t, "threads.modify", recs[1]["op"])
}

func TestNotFoundIsLoggedAtDebug(t *testing.T) {
	e, logs := newLoggedEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/threads/gone", 404, apiError(404, "notFound", "Requested entity was not found."))
	_, err := e.c.GetThreadFull(context.Background(), testAccount, "gone")
	require.ErrorIs(t, err, gmail.ErrNotFound)
	recs := logs.records(t)
	require.Len(t, recs, 1)
	assert.Equal(t, "DEBUG", recs[0]["level"])
}

func TestUntrustedFieldsAreSanitized(t *testing.T) {
	e, logs := newLoggedEnv(t)
	longMsg := strings.Repeat("あ", 2000)
	e.f.HandleJSON("GET "+apiBase+"/profile", 403, googleError(403, "PERMISSION_DENIED\n{\"level\":\"ERROR\"}",
		"line1\r\n{\"level\":\"ERROR\",\"msg\":\"forged\"}\n"+longMsg+" owner@example.com",
		[]string{"bad reason\nINJECTED", "forbidden", "forbidden", strings.Repeat("x", 100)},
		[]map[string]any{errorInfo("OK_REASON-1.2", map[string]any{"service": "gmail.googleapis.com\nINJECTED"})}))

	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, gmail.ErrGeneric)
	assert.NotContains(t, err.Error(), "\n")
	assert.NotContains(t, err.Error(), "\r")
	assert.NotContains(t, err.Error(), "owner@example.com")

	ae, ok := gmail.AsAPIError(err)
	require.True(t, ok)
	assert.Empty(t, ae.State, "文字種を満たさない error.status は載せない")
	assert.Equal(t, []string{"forbidden", "OK_REASON-1.2"}, ae.Reasons, "不正な文字種・長すぎる reason は捨て、重複は 1 つにする")
	assert.LessOrEqual(t, len([]rune(ae.Message)), googleerr.MaxMessageRune+1)
	assert.NotContains(t, ae.Metadata["service"], "\n")

	recs := logs.records(t) // 1 件が 1 行に収まっている（偽のログ行を作れない）
	require.Len(t, recs, 1)
	assert.Equal(t, "WARN", recs[0]["level"])
}

func TestHugeNonJSONBodyIsTruncated(t *testing.T) {
	e, logs := newLoggedEnv(t)
	html := "<html><body>" + strings.Repeat("Bad Gateway ", 200000) + leakedToken + "</body></html>"
	e.f.Handle("GET "+apiBase+"/profile", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(403)
		_, _ = w.Write([]byte(html))
	})

	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, gmail.ErrGeneric)
	assert.Contains(t, err.Error(), "HTTP 403")
	assert.Contains(t, err.Error(), "Bad Gateway")
	assert.Less(t, len(err.Error()), 1024, "本文の先頭の一部だけを載せる")
	assert.Less(t, len(logs.String()), 2048)
	assert.NotContains(t, logs.String(), leakedToken)
}

func TestHugeJSONBodyIsNotParsed(t *testing.T) {
	e, _ := newLoggedEnv(t)
	body := googleError(403, "PERMISSION_DENIED", "disabled", []string{"accessNotConfigured"},
		[]map[string]any{errorInfo("SERVICE_DISABLED", map[string]any{"pad": strings.Repeat("a", googleerr.MaxParseBody)})})
	e.f.HandleJSON("GET "+apiBase+"/profile", 403, body)

	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, gmail.ErrGeneric)
	ae, ok := gmail.AsAPIError(err)
	require.True(t, ok)
	// 上限を超える本文は自前では解析せず、ライブラリが解析済みの errors[].reason だけを使う。
	assert.Equal(t, []string{"accessNotConfigured"}, ae.Reasons)
}
