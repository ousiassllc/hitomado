package oauth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/logging"
)

// logRecords はテスト用 logger（logging と同じ伏せ処理つき）が書いた JSON Lines を読む。
func logRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec), line)
		out = append(out, rec)
	}
	return out
}

func stageRecord(t *testing.T, recs []map[string]any, stage string) map[string]any {
	t.Helper()
	for _, r := range recs {
		if r["stage"] == stage {
			return r
		}
	}
	require.Failf(t, "stage not logged", "stage=%s records=%v", stage, recs)
	return nil
}

func newLogged(t *testing.T) (*fakeGoogle, *bytes.Buffer) {
	t.Helper()
	g := newFakeGoogle(t)
	buf := &bytes.Buffer{}
	g.logger = logging.NewWriter(buf, slog.LevelInfo, logging.Meta{Version: "test"})
	return g, buf
}

func TestAuthorizeLogsEachStageOnSuccess(t *testing.T) {
	g, buf := newLogged(t)
	b := &browser{g: g, t: t, behave: okCallback}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.NoError(t, err)

	var stages []string
	for _, r := range logRecords(t, buf) {
		assert.Equal(t, "INFO", r["level"])
		stages = append(stages, r["stage"].(string))
	}
	assert.Equal(t, []string{"listening", "browser_opened", "callback_received", "completed"}, stages)
	for _, secret := range []string{testRefresh, testCode, testClientSecret, b.params.State, "access-token-value"} {
		assert.NotContains(t, buf.String(), secret)
	}
}

func TestAuthorizeLogsCallbackErrorCode(t *testing.T) {
	g, buf := newLogged(t)
	b := &browser{g: g, t: t, behave: func(t *testing.T, p authParams) {
		callback(t, p.Redirect, url.Values{"error": {"server_error"}, "state": {p.State}})
	}}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)

	r := stageRecord(t, logRecords(t, buf), "callback")
	assert.Equal(t, "ERROR", r["level"])
	assert.Equal(t, "server_error", r["oauth_error"])
}

func TestAuthorizeAccessDeniedIsLoggedAsWarning(t *testing.T) {
	g, buf := newLogged(t)
	b := &browser{g: g, t: t, behave: func(t *testing.T, p authParams) {
		callback(t, p.Redirect, url.Values{"error": {"access_denied"}, "state": {p.State}})
	}}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)

	r := stageRecord(t, logRecords(t, buf), "callback")
	assert.Equal(t, "WARN", r["level"])
	assert.Equal(t, "access_denied", r["oauth_error"])
}

// コールバックの error は形式を検査し、コードの形でない値（任意の文字列）はログへ書かない。
func TestAuthorizeDoesNotLogUnsafeCallbackErrorValue(t *testing.T) {
	g, buf := newLogged(t)
	b := &browser{g: g, t: t, behave: func(t *testing.T, p authParams) {
		callback(t, p.Redirect, url.Values{"error": {"Bad Value 1//FAKEREFRESHTOKENVALUE0123456789"}, "state": {p.State}})
	}}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)

	r := stageRecord(t, logRecords(t, buf), "callback")
	assert.Equal(t, "", r["oauth_error"])
	assert.NotContains(t, buf.String(), "Bad Value")
	assert.NotContains(t, buf.String(), "FAKEREFRESHTOKEN")
}

func TestAuthorizeLogsTokenExchangeStatusAndCode(t *testing.T) {
	g, buf := newLogged(t)
	g.tokenStatus = http.StatusBadRequest // 既定の本文は invalid_grant。error_description にはトークンが入っている
	b := &browser{g: g, t: t, behave: okCallback}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)

	r := stageRecord(t, logRecords(t, buf), "token_exchange")
	assert.Equal(t, "ERROR", r["level"])
	assert.InDelta(t, 400, r["status"], 0)
	assert.Equal(t, "invalid_grant", r["oauth_error"])
	assert.Contains(t, r["chain"], "httpError")
	for _, secret := range []string{testRefresh, testCode, testClientSecret, b.params.State} {
		assert.NotContains(t, buf.String(), secret)
	}
}

func TestAuthorizeLogsUserinfoFailure(t *testing.T) {
	g, buf := newLogged(t)
	g.userinfoEmail = ""
	b := &browser{g: g, t: t, behave: okCallback}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)

	r := stageRecord(t, logRecords(t, buf), "userinfo")
	assert.Equal(t, "ERROR", r["level"])
}

func TestAuthorizeLogsBrowserOpenFailureWithCause(t *testing.T) {
	g, buf := newLogged(t)
	b := &browser{g: g, t: t, openErr: errors.New("no browser")}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)

	r := stageRecord(t, logRecords(t, buf), "open_browser")
	assert.Equal(t, "no browser", r["err"])
	assert.Equal(t, "*errors.errorString", r["chain"])
}

func TestAuthorizeLogsTimeoutAsWarning(t *testing.T) {
	g, buf := newLogged(t)
	b := &browser{g: g, t: t}

	_, err := g.flow(b.open, 50*time.Millisecond).Authorize(context.Background(), cfg)
	require.Error(t, err)

	r := stageRecord(t, logRecords(t, buf), "wait_callback")
	assert.Equal(t, "WARN", r["level"])
}

func TestAuthorizeLogsScopeDenial(t *testing.T) {
	g, buf := newLogged(t)
	g.scope = "openid email profile"
	b := &browser{g: g, t: t, behave: okCallback}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)

	r := stageRecord(t, logRecords(t, buf), "scope")
	assert.Equal(t, "WARN", r["level"])
	assert.Equal(t, "openid email profile", r["grantedScope"])
}
