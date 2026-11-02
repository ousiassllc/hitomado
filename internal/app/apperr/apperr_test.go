package apperr_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/gmail"
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
