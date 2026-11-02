package gmail

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
)

const secretBody = "SECRET-BODY-should-never-leak"

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

func TestRotatedRefreshTokenIsPersisted(t *testing.T) {
	e := newTestEnv(t)
	e.f.Token = func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "rt-rotated"})
	}
	e.f.HandleJSON("GET "+apiBase+"/profile", 200, profileOK())
	_, err := e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	got, err := e.store.Get(secrets.RefreshTokenKey(testAccount))
	require.NoError(t, err)
	assert.Equal(t, "rt-rotated", got)
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
			writeJSON(w, 401, apiError(401, "authError", secretBody))
			return
		}
		writeJSON(w, 200, profileOK())
	})
	_, err := e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	assert.Equal(t, 2, e.f.TokenRequests(), "401 のあとアクセストークンを取り直す")

	e.f.HandleJSON("GET "+apiBase+"/profile", 401, apiError(401, "authError", secretBody))
	_, err = e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrNeedsReauth)
	assert.NotContains(t, err.Error(), secretBody)
}

func TestRateLimitBackoffGrowsThenGivesUp(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 429, apiError(429, "rateLimitExceeded", secretBody))

	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrRateLimited)
	assert.NotContains(t, err.Error(), secretBody)
	assert.Equal(t, []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second,
	}, e.sleeper.recorded())
	assert.Len(t, e.f.Requests("GET", apiBase+"/profile"), maxRetries+1)
}

func TestBackoffCapsAtSixtySeconds(t *testing.T) {
	assert.Equal(t, time.Second, backoffDelay(0))
	assert.Equal(t, 32*time.Second, backoffDelay(5))
	assert.Equal(t, 60*time.Second, backoffDelay(6))
	assert.Equal(t, 60*time.Second, backoffDelay(80))
}

func TestFullJitterStaysWithinBound(t *testing.T) {
	for range 200 {
		d := fullJitter(4 * time.Second)
		assert.GreaterOrEqual(t, d, time.Duration(0))
		assert.LessOrEqual(t, d, 4*time.Second)
	}
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
	e.f.HandleJSON("GET "+apiBase+"/profile", 403, apiError(403, "insufficientPermissions", "no"))
	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrGeneric)
	assert.Empty(t, e.sleeper.recorded())
	assert.Len(t, e.f.Requests("GET", apiBase+"/profile"), 1)
}

func TestServerErrorRetriesThenGeneric(t *testing.T) {
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 503, apiError(503, "backendError", secretBody))
	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, ErrGeneric)
	assert.NotContains(t, err.Error(), secretBody)
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
	e.f.HandleJSON("POST "+apiBase+"/messages/send", 500, apiError(500, "backendError", "oops"))
	_, err := e.c.Send(context.Background(), testAccount, []byte("x"), "")
	require.ErrorIs(t, err, ErrGeneric)
	assert.Empty(t, e.sleeper.recorded())
	assert.Len(t, e.f.Requests("POST", apiBase+"/messages/send"), 1)
}

func TestSendRetriesRateLimit(t *testing.T) {
	e := newTestEnv(t)
	var calls atomic.Int32
	e.f.Handle("POST "+apiBase+"/messages/send", func(w http.ResponseWriter, _ *http.Request) {
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
