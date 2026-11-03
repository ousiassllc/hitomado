package siwctests

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

func TestAccessTokenRefreshesNearExpiry(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)

	e.clk.Advance(30 * time.Minute)
	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok, "期限まで余裕があれば更新しない")
	assert.Len(t, e.f.tokenFormsCopy(), 1)

	e.clk.Advance(30 * time.Minute) // 期限の 60 秒以内
	tok, err = e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess+"-refreshed-1", tok)
	forms := e.f.tokenFormsCopy()
	require.Len(t, forms, 2)
	assert.Equal(t, "refresh_token", forms[1].Get("grant_type"))
	assert.Equal(t, issuedClient, forms[1].Get("client_id"), "更新には発行された client_id を使う")
	assert.Equal(t, secretRefresh, forms[1].Get("refresh_token"))
	assert.Equal(t, "https://api.openai.com/v1", forms[1].Get("resource"))
	assert.False(t, forms[1].Has("scope"), "scope は付けない（付与済みを維持する）")

	// 置き換えられたリフレッシュトークンが保存され、次の更新で使われる。
	got, err := e.sec.Get(secrets.ChatGPTTokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, secretRefresh+"-rotated-1", got)
	e.clk.Advance(time.Hour)
	_, err = e.m.AccessToken(t.Context())
	require.NoError(t, err)
	forms = e.f.tokenFormsCopy()
	assert.Equal(t, secretRefresh+"-rotated-1", forms[2].Get("refresh_token"))
}

func TestConcurrentAccessTokenRefreshesOnce(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	e.clk.Advance(2 * time.Hour)
	e.f.mu.Lock()
	e.f.refreshDelay = 50 * time.Millisecond
	e.f.mu.Unlock()

	var wg sync.WaitGroup
	toks := make([]string, 5)
	for i := range toks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := e.m.AccessToken(t.Context())
			assert.NoError(t, err)
			toks[i] = tok
		}()
	}
	wg.Wait()
	assert.Len(t, e.f.tokenFormsCopy(), 2, "ログイン 1 回＋更新 1 回（回転するトークンを競合させない）")
	for _, tok := range toks {
		assert.Equal(t, secretAccess+"-refreshed-1", tok)
	}
}

func TestRefreshFailureModes(t *testing.T) {
	for _, code := range []string{"invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused"} {
		t.Run(code, func(t *testing.T) {
			e := newEnv(t, nil, nil)
			e.signIn(t)
			e.clk.Advance(2 * time.Hour)
			e.f.mu.Lock()
			e.f.refreshStatus, e.f.refreshBody = http.StatusBadRequest, fmt.Sprintf(`{"error":%q}`, code)
			e.f.mu.Unlock()

			_, err := e.m.AccessToken(t.Context())
			require.ErrorIs(t, err, siwc.ErrReauthRequired, "再ログインを案内する")
			_, serr := e.sec.Get(secrets.ChatGPTTokenKey("refresh"))
			assert.ErrorIs(t, serr, secrets.ErrNotFound, "使えないトークンは消す")
			st, _ := e.m.Status(t.Context())
			assert.False(t, st.SignedIn)
			_, err = e.m.AccessToken(t.Context())
			assert.ErrorIs(t, err, siwc.ErrNotSignedIn)
			assert.Equal(t, issuedClient, e.reg(t).ClientID, "登録は残す（同じクライアントで再ログインできる）")
		})
	}

	t.Run("一時的な失敗ではトークンを消さない", func(t *testing.T) {
		e := newEnv(t, nil, nil)
		e.signIn(t)
		e.clk.Advance(2 * time.Hour)
		e.f.mu.Lock()
		e.f.refreshStatus, e.f.refreshBody = http.StatusServiceUnavailable, `{"detail":"down"}`
		e.f.mu.Unlock()
		_, err := e.m.AccessToken(t.Context())
		require.Error(t, err)
		assert.NotErrorIs(t, err, siwc.ErrReauthRequired)
		got, serr := e.sec.Get(secrets.ChatGPTTokenKey("refresh"))
		require.NoError(t, serr)
		assert.Equal(t, secretRefresh, got)

		e.f.mu.Lock()
		e.f.refreshStatus = 0
		e.f.mu.Unlock()
		_, err = e.m.AccessToken(t.Context())
		assert.NoError(t, err, "復旧したら更新できる")
	})
	t.Run("invalid_client は再ログインではなく設定の失敗", func(t *testing.T) {
		e := newEnv(t, nil, nil)
		e.signIn(t)
		e.clk.Advance(2 * time.Hour)
		e.f.mu.Lock()
		e.f.refreshStatus, e.f.refreshBody = http.StatusUnauthorized, `{"error":"invalid_client"}`
		e.f.mu.Unlock()
		_, err := e.m.AccessToken(t.Context())
		assert.ErrorIs(t, err, siwc.ErrFailed)
		assert.NotErrorIs(t, err, siwc.ErrReauthRequired)
	})
}

func TestForceRefresh(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	tok, err := e.m.ForceRefresh(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess+"-refreshed-1", tok, "期限内でも更新する")
}

func TestNotSignedIn(t *testing.T) {
	e := newEnv(t, nil, nil)
	_, err := e.m.AccessToken(t.Context())
	assert.ErrorIs(t, err, siwc.ErrNotSignedIn)
	_, err = e.m.ForceRefresh(t.Context())
	assert.ErrorIs(t, err, siwc.ErrNotSignedIn)
}

func TestSignOutRevokesThenClears(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	revoked, err := e.m.SignOut(t.Context())
	require.NoError(t, err)
	assert.True(t, revoked)
	forms := e.f.revokeFormsCopy()
	require.Len(t, forms, 1)
	assert.Equal(t, secretRefresh, forms[0].Get("token"))
	assert.Equal(t, "refresh_token", forms[0].Get("token_type_hint"))
	assert.Equal(t, issuedClient, forms[0].Get("client_id"))
	for _, k := range []string{"access", "refresh", "id"} {
		_, err := e.sec.Get(secrets.ChatGPTTokenKey(k))
		assert.ErrorIs(t, err, secrets.ErrNotFound, k)
	}
	st, _ := e.m.Status(t.Context())
	assert.False(t, st.SignedIn)
	assert.Equal(t, issuedClient, e.reg(t).ClientID, "登録（クライアントとホスト）は残す")
	assert.NotEmpty(t, e.reg(t).HostID)

	revoked, err = e.m.SignOut(t.Context())
	require.NoError(t, err, "ログインしていなくてもエラーにしない")
	assert.True(t, revoked)
}

func TestSignOutClearsLocallyEvenWhenRevocationFails(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	e.f.mu.Lock()
	e.f.revokeStatus = http.StatusBadGateway
	e.f.mu.Unlock()
	revoked, err := e.m.SignOut(t.Context())
	require.NoError(t, err)
	assert.False(t, revoked, "失効を確認できなかったことを伝える")
	assert.Len(t, e.f.revokeFormsCopy(), 3, "5xx は再試行する")
	_, serr := e.sec.Get(secrets.ChatGPTTokenKey("refresh"))
	assert.ErrorIs(t, serr, secrets.ErrNotFound)
}
