package siwctests

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/siwc"
)

// ログインの失敗は、どの段階で何が起きたかをログに残す（#185。画面には「予期しないエラー」などとしか出ないため）。

func TestSignInLogsEachStageOnSuccess(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)

	logs := e.logs.String()
	for _, stage := range []string{"stage=listening", "stage=browser_opened", "stage=callback_received", "stage=completed"} {
		assert.Contains(t, logs, stage)
	}
	assert.NotContains(t, logs, "level=ERROR")
	assert.NotContains(t, logs, "level=WARN")
}

func TestSignInLogsTokenExchangeStatusAndCode(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.tokenStatus, e.f.tokenBody = http.StatusBadRequest, `{"error":"invalid_request","error_description":"bad `+secretAccess+`"}`

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.Error(t, err)

	logs := e.logs.String()
	assert.Contains(t, logs, "level=ERROR")
	assert.Contains(t, logs, "stage=token_exchange")
	assert.Contains(t, logs, "status=400")
	assert.Contains(t, logs, "oauth_error=invalid_request")
	assert.NotContains(t, logs, secretAccess)
}

func TestSignInKeepsTheServerErrorWhenTheCodeIsUnusable(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.tokenStatus, e.f.tokenBody = http.StatusBadRequest, `{"error":"invalid_grant"}`

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})

	require.ErrorIs(t, err, siwc.ErrInvalidResponse)
	assert.Contains(t, e.logs.String(), "oauth_error=invalid_grant")
	assert.Contains(t, e.logs.String(), "status=400")
}

func TestSignInLogsCallbackErrorCode(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.callbackExtras = url.Values{"error": {"server_error"}}

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})

	require.ErrorIs(t, err, siwc.ErrFailed)
	logs := e.logs.String()
	assert.Contains(t, logs, "stage=callback")
	assert.Contains(t, logs, "oauth_error=server_error")
}

func TestSignInDoesNotLogUnsafeCallbackErrorValue(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.callbackExtras = url.Values{"error": {"Bad Value " + secretRefresh}}

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})

	require.ErrorIs(t, err, siwc.ErrFailed)
	assert.NotContains(t, e.logs.String(), "Bad Value")
	assert.NotContains(t, e.logs.String(), secretRefresh)
}

func TestSignInAccessDeniedIsAWarning(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.callbackExtras = url.Values{"error": {"access_denied"}}

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})

	require.ErrorIs(t, err, siwc.ErrDenied)
	logs := e.logs.String()
	assert.Contains(t, logs, "level=WARN")
	assert.Contains(t, logs, "oauth_error=access_denied")
	assert.NotContains(t, logs, "level=ERROR")
}

func TestSignInLogsAndKeepsTheBrowserOpenCause(t *testing.T) {
	e := newEnv(t, nil, func(o *siwc.Options) {
		o.OpenBrowser = func(string) error { return errors.New("no activity to handle the intent") }
	})

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})

	require.ErrorIs(t, err, siwc.ErrFailed)
	assert.Contains(t, err.Error(), "no activity to handle the intent")
	logs := e.logs.String()
	assert.Contains(t, logs, "stage=open_browser")
	assert.Contains(t, logs, "no activity to handle the intent")
}

// ID トークンの検証の詳細は戻り値のエラーには載せず、ログにだけ残す（トークンの中身を含み得るため）。
func TestSignInLogsTheIdentityCheckFailure(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.idTokenAud = "someone-else"

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})

	require.ErrorIs(t, err, siwc.ErrInvalidResponse)
	assert.NotContains(t, err.Error(), "someone-else")
	logs := e.logs.String()
	assert.Contains(t, logs, "stage=identity_check")
	assert.Contains(t, logs, "cause=")
}

func TestSignInLogsUnexpectedFailureWithTheOriginalError(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.tokenStatus, e.f.tokenBody = http.StatusInternalServerError, ``

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})

	require.Error(t, err)
	assert.Contains(t, e.logs.String(), fmt.Sprintf("status=%d", http.StatusInternalServerError))
	assert.Contains(t, e.logs.String(), "chain=")
}
