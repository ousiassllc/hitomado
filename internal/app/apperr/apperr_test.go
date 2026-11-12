package apperr_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/gmail/googleerr"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/store"
)

func TestFromInfraMapsSentinels(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		code      string
		retryable bool
	}{
		{"needs reauth", gmail.ErrNeedsReauth, apperr.NeedsReauth, false},
		{"client rejected", gmail.ErrClientRejected, apperr.OAuthClientRejected, false},
		{"oauth client rejected", oauth.ErrClientRejected, apperr.OAuthClientRejected, false},
		{"offline", gmail.ErrOffline, apperr.Offline, true},
		{"send uncertain", gmail.ErrSendUncertain, apperr.SendUncertain, false},
		{"net error", &net.DNSError{Err: "no such host"}, apperr.Offline, false},
		{"rate limited", gmail.ErrRateLimited, apperr.RateLimited, true},
		{"gmail not found", gmail.ErrNotFound, apperr.NotFound, false},
		{"store not found", store.ErrNotFound, apperr.NotFound, false},
		{"attachment gone", gmail.ErrAttachmentGone, apperr.AttachmentIO, false},
		{"oauth client in use", store.ErrOAuthClientUse, apperr.OAuthClientInUse, false},
		{"oauth cancelled", oauth.ErrCancelled, apperr.Cancelled, false},
		{"context canceled", context.Canceled, apperr.Cancelled, false},
		{"oauth denied", oauth.ErrDenied, apperr.OAuthDenied, false},
		{"oauth timeout", oauth.ErrTimeout, apperr.OAuthTimeout, true},
		{"ai unauthorized", ai.ErrUnauthorized, apperr.AIFailed, false},
		{"ai rate limited", ai.ErrRateLimited, apperr.RateLimited, true},
		{"ai failed", ai.ErrFailed, apperr.AIFailed, false},
		{"deadline", context.DeadlineExceeded, apperr.Offline, true},
		{"gmail generic", gmail.ErrGeneric, apperr.GmailError, false},
		{"unknown", errors.New("boom"), apperr.Internal, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("layer: %w", tc.err)
			e := apperr.FromInfra(wrapped, "acc1")
			require.NotNil(t, e)
			assert.Equal(t, tc.code, e.Code)
			assert.Equal(t, "acc1", e.AccountID)
			assert.NotEmpty(t, e.Message)
			assert.ErrorIs(t, e, tc.err, "元のエラーは Unwrap で辿れる")
			if tc.code == apperr.Offline || tc.code == apperr.RateLimited || tc.code == apperr.OAuthTimeout {
				assert.True(t, e.Retryable)
			}
		})
	}
}

func TestIsPermanent(t *testing.T) {
	assert.True(t, apperr.IsPermanent(gmail.ErrNeedsReauth))
	assert.True(t, apperr.IsPermanent(gmail.ErrClientRejected))
	assert.True(t, apperr.IsPermanent(gmail.ErrNotFound))
	assert.False(t, apperr.IsPermanent(gmail.ErrRateLimited))
	assert.False(t, apperr.IsPermanent(gmail.ErrOffline))
	assert.False(t, apperr.IsPermanent(gmail.ErrGeneric))
}

func TestFromInfraCancelBeatsNetError(t *testing.T) {
	// url.Error はキャンセルでも net.Error を満たすため、OFFLINE と誤判定しないこと。
	err := &url.Error{Op: "Get", URL: "https://example.invalid", Err: context.Canceled}
	var ne net.Error
	require.ErrorAs(t, err, &ne)
	assert.Equal(t, apperr.Cancelled, apperr.FromInfra(err, "").Code)
	assert.Equal(t, apperr.Cancelled, apperr.FromInfra(fmt.Errorf("x: %w", oauth.ErrCancelled), "").Code)
}

func TestFromInfraPassesThroughAppError(t *testing.T) {
	orig := apperr.New(apperr.Validation, "入力が不正です").WithDetail("k", 1)
	got := apperr.FromInfra(fmt.Errorf("ctx: %w", orig), "acc1")
	assert.Same(t, orig, got)
	assert.Empty(t, got.AccountID, "既存の AppError は書き換えない")
	assert.Nil(t, apperr.FromInfra(nil, "acc1"))
}

func TestFromInfraOmitsAccountIDWhenEmpty(t *testing.T) {
	assert.Empty(t, apperr.FromInfra(gmail.ErrGeneric, "").AccountID)
}

func TestNewRetryableDefaults(t *testing.T) {
	assert.True(t, apperr.New(apperr.Offline, "x").Retryable)
	assert.True(t, apperr.New(apperr.RateLimited, "x").Retryable)
	assert.False(t, apperr.New(apperr.GmailError, "x").Retryable)
}

func TestMessagesDoNotLeakCause(t *testing.T) {
	secret := "ya29.SECRET-TOKEN subject=極秘 to=boss@example.com" //nolint:gosec // 漏えい検査用のダミー値
	for _, base := range []error{gmail.ErrGeneric, gmail.ErrNeedsReauth, errors.New("x"), ai.ErrFailed} {
		e := apperr.FromInfra(fmt.Errorf("%s: %w", secret, base), "acc1")
		assert.NotContains(t, e.Message, "SECRET")
		assert.NotContains(t, e.Message, "boss@example.com")
		assert.NotContains(t, string(apperr.Marshal(fmt.Errorf("%s: %w", secret, base))), "SECRET")
		assert.NotContains(t, fmt.Sprint(apperr.ToMap(fmt.Errorf("%s: %w", secret, base))), "SECRET")
	}
}

func TestMarshal(t *testing.T) {
	b := apperr.Marshal(apperr.Wrap(apperr.RateLimited, "しばらく待ってください", gmail.ErrRateLimited).ForAccount("a1").WithDetail("failedRefs", []string{"x"}))
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	assert.Equal(t, "RATE_LIMITED", m["code"])
	assert.Equal(t, "しばらく待ってください", m["message"])
	assert.Equal(t, true, m["retryable"])
	assert.Equal(t, "a1", m["accountId"])
	assert.Equal(t, map[string]any{"failedRefs": []any{"x"}}, m["details"])
	assert.NotContains(t, m, "cause", "cause は JSON に出さない")

	m = nil
	require.NoError(t, json.Unmarshal(apperr.Marshal(errors.New("boom")), &m))
	assert.Equal(t, "INTERNAL", m["code"])
	assert.NotContains(t, m, "accountId")
	assert.NotContains(t, m, "details")
}

func TestToMap(t *testing.T) {
	assert.Nil(t, apperr.ToMap(nil))
	m := apperr.ToMap(apperr.New(apperr.NeedsReauth, "再接続").ForAccount("a1").WithDetail("d", 1))
	assert.Equal(t, map[string]any{
		"code": "NEEDS_REAUTH", "message": "再接続", "retryable": false, "accountId": "a1", "details": map[string]any{"d": 1},
	}, m)
	m = apperr.ToMap(gmail.ErrOffline)
	assert.Equal(t, "OFFLINE", m["code"])
	assert.Equal(t, true, m["retryable"])
	assert.NotContains(t, m, "accountId")
	assert.NotContains(t, m, "details")
}

func TestWithDetailDoesNotMutate(t *testing.T) {
	orig := apperr.New(apperr.GmailError, "x")
	d := orig.WithDetail("k", "v")
	assert.Nil(t, orig.Details)
	assert.Equal(t, "v", d.Details["k"])
}

// 利用枠の超過だけが details.reason を持つ（画面が Manage usage を主なアクションにした表示へ切り替える。メッセージの文言には頼らない）。
func TestUsageLimitCarriesAReasonForTheUI(t *testing.T) {
	e := apperr.FromInfra(fmt.Errorf("%w: secret-value", ai.ErrUsageLimit), "")
	assert.Equal(t, apperr.AIFailed, e.Code)
	assert.Equal(t, apperr.ReasonUsageLimit, e.Details[apperr.DetailReason])
	assert.True(t, errors.Is(e, ai.ErrUsageLimit), "元のエラーは保持する")
	b, err := json.Marshal(e)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"reason":"usage_limit_exceeded"`)
	assert.NotContains(t, string(b), "secret-value")

	for _, other := range []error{ai.ErrNotEligible, ai.ErrFailed, ai.ErrUnauthorized, ai.ErrRateLimited} {
		assert.Empty(t, apperr.FromInfra(other, "").Details, other.Error())
	}
}

// Gmail API が返した reason に応じて、利用者が自分で直せる案内に変わる（#96）。
func TestFromInfraGmailAPIReasons(t *testing.T) {
	wrap := func(kind error, status int, reasons ...string) error {
		return fmt.Errorf("sync: %w", &gmail.APIError{Info: googleerr.Info{Status: status, Reasons: reasons}, Op: "getProfile", Kind: kind})
	}
	tests := []struct {
		name       string
		err        error
		code       string
		contains   string
		detailWant any
	}{
		{"API 無効（SERVICE_DISABLED）", wrap(gmail.ErrGeneric, 403, "SERVICE_DISABLED"), apperr.GmailError, "Gmail API が有効になっていません", apperr.ReasonGmailAPIDisabled},
		{"API 無効（accessNotConfigured）", wrap(gmail.ErrGeneric, 403, "accessNotConfigured"), apperr.GmailError, "Gmail API が有効になっていません", apperr.ReasonGmailAPIDisabled},
		{"スコープ不足（ACCESS_TOKEN_SCOPE_INSUFFICIENT）", wrap(gmail.ErrNeedsReauth, 403, "ACCESS_TOKEN_SCOPE_INSUFFICIENT"), apperr.NeedsReauth, "権限（スコープ）が足りません", apperr.ReasonScopeInsufficient},
		{"スコープ不足（insufficientPermissions）", wrap(gmail.ErrNeedsReauth, 403, "insufficientPermissions"), apperr.NeedsReauth, "再接続して", apperr.ReasonScopeInsufficient},
		{"401", wrap(gmail.ErrNeedsReauth, 401, "authError"), apperr.NeedsReauth, "接続が切れました", nil},
		{"その他の 403 は HTTP ステータスと reason を添える", wrap(gmail.ErrGeneric, 403, "forbidden"), apperr.GmailError, "HTTP 403、理由: forbidden", nil},
		{"reason なしは HTTP ステータスだけ", wrap(gmail.ErrGeneric, 500), apperr.GmailError, "HTTP 500", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := apperr.FromInfra(tc.err, "acc1")
			assert.Equal(t, tc.code, e.Code)
			assert.Contains(t, e.Message, tc.contains)
			assert.Equal(t, tc.detailWant, e.Details[apperr.DetailReason])
			assert.False(t, e.Retryable)
			assert.Equal(t, "acc1", e.AccountID)
		})
	}
}

// Google が返した message（トークンが混ざり得る）は画面向けメッセージに載せない。
func TestGmailAPIMessageIsNotShownToUser(t *testing.T) {
	ae := &gmail.APIError{Info: googleerr.Info{Status: 403, Reasons: []string{"forbidden"}, Message: "Bearer ya29.DUMMY-token"}, Op: "getProfile", Kind: gmail.ErrGeneric} //nolint:gosec // 漏えい検査用のダミー値
	e := apperr.FromInfra(ae, "")
	assert.NotContains(t, e.Message, "ya29")
	assert.NotContains(t, string(apperr.Marshal(ae)), "ya29")
}

// 分類できないエラー（INTERNAL）は、画面・JSON には原因を出さず、ログへ元のエラーと型のつながりを残す（#184）。
func TestFromInfraLogsTheCauseOfInternal(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(logging.NewWriter(&buf, slog.LevelInfo, logging.Meta{Version: "test"}))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cause := fmt.Errorf("oauth: ブラウザを開けませんでした: %w", errors.New("intent failed"))
	e := apperr.FromInfra(cause, "acc1")

	assert.Equal(t, apperr.Internal, e.Code)
	assert.NotContains(t, e.Message, "intent failed")
	assert.NotContains(t, string(apperr.Marshal(cause)), "intent failed")
	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes()[:bytes.IndexByte(buf.Bytes(), '\n')], &rec))
	assert.Equal(t, "ERROR", rec["level"])
	assert.Equal(t, "internal error", rec["msg"])
	assert.Equal(t, "oauth: ブラウザを開けませんでした: intent failed", rec["err"])
	assert.Equal(t, "*fmt.wrapError > *errors.errorString", rec["chain"])
	assert.Equal(t, "acc1", rec["accountId"])
}

// 分類できたエラーは INTERNAL のログを出さない（想定内の失敗でログを埋めない）。
func TestFromInfraDoesNotLogClassifiedErrors(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(logging.NewWriter(&buf, slog.LevelInfo, logging.Meta{Version: "test"}))
	t.Cleanup(func() { slog.SetDefault(prev) })

	_ = apperr.FromInfra(gmail.ErrOffline, "")
	_ = apperr.FromInfra(apperr.New(apperr.Internal, "既に AppError"), "")

	assert.Empty(t, buf.String())
}
