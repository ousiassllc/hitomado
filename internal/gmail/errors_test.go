package gmail

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/ousiassllc/hitomado/internal/gmail/tokenauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

const secretBody = "SECRET-BODY-should-never-leak"

// Google のエラー message に混ざり得るトークン風のダミー値。伏せ字処理で消えること。
const leakedToken = "ya29.DUMMY-access-token_0123456789abcdef" //nolint:gosec // 漏えい検査用のダミー値

func profileOK() map[string]any {
	return map[string]any{"emailAddress": "a@example.com", "historyId": "7"}
}

func TestInvalidGrantIsNeedsReauth(t *testing.T) {
	e := newTestEnv(t)
	e.f.Token = func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 400, map[string]any{"error": "invalid_grant", "error_description": secretBody})
	}
	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrNeedsReauth)
	assert.NotContains(t, err.Error(), secretBody)
	assert.Empty(t, e.sleeper.recorded())
	assert.Empty(t, e.f.Requests("GET", apiBase+"/profile"))
}

func TestNeedsReauthDropsCacheSoReconnectedTokenIsUsed(t *testing.T) {
	e := newTestEnv(t)
	e.f.Token = func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.PostForm.Get("refresh_token") == "rt-1" {
			writeJSON(w, 400, map[string]any{"error": "invalid_grant"})
			return
		}
		writeJSON(w, 200, map[string]any{"access_token": "fresh", "token_type": "Bearer", "expires_in": 3600})
	}
	e.f.HandleJSON("GET "+apiBase+"/profile", 200, profileOK())

	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrNeedsReauth)

	// 再接続で新しいリフレッシュトークンが保存されたあとは、再試行で成功する。
	require.NoError(t, e.store.Set(secrets.RefreshTokenKey(testAccount), "rt-2"))
	_, err = e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	assert.Equal(t, "Bearer fresh", e.f.Requests("GET", apiBase+"/profile")[0].Header.Get("Authorization"))
}

func TestMissingRefreshTokenIsNeedsReauth(t *testing.T) {
	e := newTestEnv(t)
	require.NoError(t, e.store.Delete(secrets.RefreshTokenKey(testAccount)))
	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrNeedsReauth)
}

func TestUnknownAccountIsGenericError(t *testing.T) {
	e := newTestEnv(t)
	_, err := e.c.Profile(context.Background(), "other")
	require.ErrorIs(t, err, ErrGeneric)
}

func TestForgetRereadsCredentials(t *testing.T) {
	e := newTestEnv(t)
	var seen []string
	e.f.Token = func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		seen = append(seen, r.PostForm.Get("refresh_token"))
		writeJSON(w, 200, map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600})
	}
	e.f.HandleJSON("GET "+apiBase+"/profile", 200, profileOK())

	_, err := e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	require.NoError(t, e.store.Set(secrets.RefreshTokenKey(testAccount), "rt-new"))
	e.c.Forget(testAccount)
	_, err = e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	assert.Equal(t, []string{"rt-1", "rt-new"}, seen)
}

func TestUnauthorizedRefreshesTokenOnceThenNeedsReauth(t *testing.T) {
	e := newTestEnv(t)
	var calls atomic.Int32
	e.f.Handle("GET "+apiBase+"/profile", func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writeJSON(w, 401, apiError(401, "authError", leakedToken))
			return
		}
		writeJSON(w, 200, profileOK())
	})
	_, err := e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	assert.Equal(t, 2, e.f.TokenRequests(), "401 のあとアクセストークンを取り直す")

	e.f.HandleJSON("GET "+apiBase+"/profile", 401, apiError(401, "authError", leakedToken))
	_, err = e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrNeedsReauth)
	assert.NotContains(t, err.Error(), leakedToken)
}

func TestRateLimitBackoffGrowsThenGivesUp(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 429, apiError(429, "rateLimitExceeded", leakedToken))

	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrRateLimited)
	assert.NotContains(t, err.Error(), leakedToken)
	assert.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second,
	}, e.sleeper.recorded())
	assert.Len(t, e.f.Requests("GET", apiBase+"/profile"), maxRetries+1)
}

func TestRateLimitRecoversAfterRetry(t *testing.T) {
	e := newTestEnv(t)
	var calls atomic.Int32
	e.f.Handle("GET "+apiBase+"/profile", func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= 2 {
			writeJSON(w, 403, apiError(403, "userRateLimitExceeded", "slow down"))
			return
		}
		writeJSON(w, 200, profileOK())
	})
	got, err := e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	assert.Equal(t, "a@example.com", got.Email)
	assert.Equal(t, []time.Duration{time.Second, 2 * time.Second}, e.sleeper.recorded())
}

func TestForbiddenWithoutRateLimitReasonIsGeneric(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 403, apiError(403, "forbidden", "no"))
	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrGeneric)
	assert.Empty(t, e.sleeper.recorded())
	assert.Len(t, e.f.Requests("GET", apiBase+"/profile"), 1)
}

func TestServerErrorRetriesThenGeneric(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 503, apiError(503, "backendError", leakedToken))
	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrGeneric)
	assert.NotContains(t, err.Error(), leakedToken)
	assert.Len(t, e.sleeper.recorded(), maxRetries)
}

func TestClientErrorIsGenericWithoutRetry(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 400, apiError(400, "invalidArgument", "bad"))
	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrGeneric)
	assert.Empty(t, e.sleeper.recorded())
}

func TestSendDoesNotRetryServerErrors(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("POST "+sendUpload, 500, apiError(500, "backendError", "oops"))
	_, err := e.c.Send(context.Background(), testAccount, []byte("x"), "")
	require.ErrorIs(t, err, ErrGeneric)
	assert.Empty(t, e.sleeper.recorded())
	assert.Len(t, e.f.Requests("POST", sendUpload), 1)
}

func TestSendRetriesRateLimit(t *testing.T) {
	e := newTestEnv(t)
	var calls atomic.Int32
	e.f.Handle("POST "+sendUpload, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writeJSON(w, 429, apiError(429, "rateLimitExceeded", "slow"))
			return
		}
		writeJSON(w, 200, map[string]any{"id": "m", "threadId": "t"})
	})
	_, err := e.c.Send(context.Background(), testAccount, []byte("x"), "")
	require.NoError(t, err)
	assert.Len(t, e.sleeper.recorded(), 1)
}

func TestContextCancelStopsBackoffImmediately(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 429, apiError(429, "rateLimitExceeded", "slow"))
	ctx, cancel := context.WithCancel(context.Background())
	e.sleeper.onSleep = func(n int) {
		if n == 2 {
			cancel()
		}
	}
	_, err := e.c.Profile(ctx, testAccount)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ErrRateLimited)
	assert.Len(t, e.sleeper.recorded(), 2)
}

func TestCancelledContextBeforeCall(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 200, profileOK())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := e.c.Profile(ctx, testAccount)
	require.ErrorIs(t, err, context.Canceled)
}

func TestNetworkFailureIsOffline(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 200, profileOK())
	_, err := e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)

	e.f.Close()
	_, err = e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrOffline)
	assert.Empty(t, e.sleeper.recorded())
}

func TestTokenEndpointUnreachableIsOffline(t *testing.T) {
	e := newTestEnv(t)
	e.f.Close()
	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrOffline)
}

func TestNewClientRequiresDependencies(t *testing.T) {
	_, err := NewClient(Options{Accounts: fakeLookup{}})
	require.Error(t, err)
	_, err = NewClient(Options{Secrets: secrets.NewMemory()})
	require.Error(t, err)
}

func TestClientImplementsForgetter(t *testing.T) {
	e := newTestEnv(t)
	var c Client = e.c
	_, ok := c.(Forgetter)
	assert.True(t, ok)
}

func TestClassifyRejectedOAuthClient(t *testing.T) {
	for _, tc := range []struct {
		code   string
		status int
	}{{"invalid_client", 401}, {"unauthorized_client", 400}, {"", 401}} {
		err := &oauth2.RetrieveError{Response: &http.Response{StatusCode: tc.status}, ErrorCode: tc.code}
		f := classify(err)
		assert.Equal(t, ErrClientRejected, f.kind, "%q/%d", tc.code, tc.status)
		assert.Equal(t, retryNone, f.retry)
	}
	assert.Equal(t, ErrNeedsReauth, classify(&oauth2.RetrieveError{Response: &http.Response{StatusCode: 400}, ErrorCode: "invalid_grant"}).kind)
}

func TestResultUnknown(t *testing.T) {
	post := func(err error) error { return &url.Error{Op: "Post", URL: "https://gmail.example/", Err: err} }
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"応答を読めず切断", post(io.EOF), true},
		{"応答待ちのタイムアウト", post(context.DeadlineExceeded), true},
		{"キャンセル", post(context.Canceled), true},
		{"読み取り中の切断", post(&net.OpError{Op: "read", Err: errors.New("connection reset by peer")}), true},
		{"接続拒否", post(&net.OpError{Op: "dial", Err: errors.New("connection refused")}), false},
		{"名前解決失敗", post(&net.DNSError{Err: "no such host", Name: "gmail.example"}), false},
		{"トークン更新の通信失敗", post(errors.Join(tokenauth.ErrRefresh, post(io.EOF))), false},
		{"応答を受け取った 5xx", &googleapi.Error{Code: 503}, false},
		{"失敗なし", nil, false},
	} {
		assert.Equal(t, tc.want, resultUnknown(tc.err), tc.name)
	}
}

// lostKeyStore は Android で Keystore の鍵が失われた（保存済みの資格情報を復号できない）状態の Store。
type lostKeyStore struct{ secrets.Store }

func (lostKeyStore) Get(string) (string, error) { return "", secrets.ErrKeyLost }

func TestKeyLostRefreshTokenIsNeedsReauth(t *testing.T) {
	e := newTestEnvWith(t, func(o *Options) { o.Secrets = lostKeyStore{o.Secrets} })
	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrNeedsReauth) // 再ログインへ（アカウントは needs_reauth になる）
	assert.Empty(t, e.f.Requests("GET", apiBase+"/profile"))
}
