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
	"github.com/ousiassllc/hitomado/internal/siwc/siwctest"
)

func TestAccessTokenRefreshesNearExpiry(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)

	e.Clk.Advance(30 * time.Minute)
	tok, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess, tok, "期限まで余裕があれば更新しない")
	assert.Len(t, e.F.TokenFormsCopy(), 1)

	e.Clk.Advance(30 * time.Minute) // 期限の 60 秒以内
	tok, err = e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess+"-refreshed-1", tok)
	forms := e.F.TokenFormsCopy()
	require.Len(t, forms, 2)
	assert.Equal(t, "refresh_token", forms[1].Get("grant_type"))
	assert.Equal(t, siwctest.IssuedClient, forms[1].Get("client_id"), "更新には発行された client_id を使う")
	assert.Equal(t, siwctest.SecretRefresh, forms[1].Get("refresh_token"))
	assert.Equal(t, "https://api.openai.com/v1", forms[1].Get("resource"))
	assert.False(t, forms[1].Has("scope"), "scope は付けない（付与済みを維持する）")

	// 置き換えられたリフレッシュトークンが保存され、次の更新で使われる。
	got, err := e.Sec.Get(siwctest.TokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretRefresh+"-rotated-1", got)
	e.Clk.Advance(time.Hour)
	_, err = e.M.AccessToken(t.Context())
	require.NoError(t, err)
	forms = e.F.TokenFormsCopy()
	assert.Equal(t, siwctest.SecretRefresh+"-rotated-1", forms[2].Get("refresh_token"))
}

func TestConcurrentAccessTokenRefreshesOnce(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)
	e.Clk.Advance(2 * time.Hour)
	e.F.Mu.Lock()
	e.F.RefreshDelay = 50 * time.Millisecond
	e.F.Mu.Unlock()

	var wg sync.WaitGroup
	toks := make([]string, 5)
	for i := range toks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := e.M.AccessToken(t.Context())
			assert.NoError(t, err)
			toks[i] = tok
		}()
	}
	wg.Wait()
	assert.Len(t, e.F.TokenFormsCopy(), 2, "ログイン 1 回＋更新 1 回（回転するトークンを競合させない）")
	for _, tok := range toks {
		assert.Equal(t, siwctest.SecretAccess+"-refreshed-1", tok)
	}
}

func TestRefreshFailureModes(t *testing.T) {
	for _, code := range []string{"invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused"} {
		t.Run(code, func(t *testing.T) {
			e := siwctest.NewEnv(t, nil, nil)
			e.SignIn(t)
			e.Clk.Advance(2 * time.Hour)
			e.F.Mu.Lock()
			e.F.RefreshStatus, e.F.RefreshBody = http.StatusBadRequest, fmt.Sprintf(`{"error":%q}`, code)
			e.F.Mu.Unlock()

			_, err := e.M.AccessToken(t.Context())
			require.ErrorIs(t, err, siwc.ErrReauthRequired, "再ログインを案内する")
			_, serr := e.Sec.Get(siwctest.TokenKey("refresh"))
			assert.ErrorIs(t, serr, secrets.ErrNotFound, "使えないトークンは消す")
			st, _ := e.M.Status(t.Context())
			assert.False(t, st.SignedIn)
			_, err = e.M.AccessToken(t.Context())
			assert.ErrorIs(t, err, siwc.ErrNotSignedIn)
			assert.Equal(t, siwctest.IssuedClient, e.Reg(t).First().ClientID, "登録は残す（同じクライアントで再ログインできる）")
		})
	}

	t.Run("一時的な失敗ではトークンを消さない", func(t *testing.T) {
		e := siwctest.NewEnv(t, nil, nil)
		e.SignIn(t)
		e.Clk.Advance(2 * time.Hour)
		e.F.Mu.Lock()
		e.F.RefreshStatus, e.F.RefreshBody = http.StatusServiceUnavailable, `{"detail":"down"}`
		e.F.Mu.Unlock()
		_, err := e.M.AccessToken(t.Context())
		require.Error(t, err)
		assert.NotErrorIs(t, err, siwc.ErrReauthRequired)
		got, serr := e.Sec.Get(siwctest.TokenKey("refresh"))
		require.NoError(t, serr)
		assert.Equal(t, siwctest.SecretRefresh, got)

		e.F.Mu.Lock()
		e.F.RefreshStatus = 0
		e.F.Mu.Unlock()
		_, err = e.M.AccessToken(t.Context())
		assert.NoError(t, err, "復旧したら更新できる")
	})
	t.Run("invalid_client は再ログインではなく設定の失敗", func(t *testing.T) {
		e := siwctest.NewEnv(t, nil, nil)
		e.SignIn(t)
		e.Clk.Advance(2 * time.Hour)
		e.F.Mu.Lock()
		e.F.RefreshStatus, e.F.RefreshBody = http.StatusUnauthorized, `{"error":"invalid_client"}`
		e.F.Mu.Unlock()
		_, err := e.M.AccessToken(t.Context())
		assert.ErrorIs(t, err, siwc.ErrFailed)
		assert.NotErrorIs(t, err, siwc.ErrReauthRequired)
	})
}

func TestForceRefresh(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)
	tok, err := e.M.ForceRefresh(t.Context())
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess+"-refreshed-1", tok, "期限内でも更新する")
}

func TestNotSignedIn(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	_, err := e.M.AccessToken(t.Context())
	assert.ErrorIs(t, err, siwc.ErrNotSignedIn)
	_, err = e.M.ForceRefresh(t.Context())
	assert.ErrorIs(t, err, siwc.ErrNotSignedIn)
}

func TestSignOutRevokesThenClears(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)
	revoked, err := e.M.SignOut(t.Context(), siwctest.IssuedClient)
	require.NoError(t, err)
	assert.True(t, revoked)
	forms := e.F.RevokeFormsCopy()
	require.Len(t, forms, 1)
	assert.Equal(t, siwctest.SecretRefresh, forms[0].Get("token"))
	assert.Equal(t, "refresh_token", forms[0].Get("token_type_hint"))
	assert.Equal(t, siwctest.IssuedClient, forms[0].Get("client_id"))
	for _, k := range []string{"access", "refresh", "id"} {
		_, err := e.Sec.Get(siwctest.TokenKey(k))
		assert.ErrorIs(t, err, secrets.ErrNotFound, k)
	}
	st, _ := e.M.Status(t.Context())
	assert.False(t, st.SignedIn)
	assert.Equal(t, siwctest.IssuedClient, e.Reg(t).First().ClientID, "登録（クライアントとホスト）は残す")
	assert.NotEmpty(t, e.Reg(t).HostID)

	revoked, err = e.M.SignOut(t.Context(), siwctest.IssuedClient)
	require.NoError(t, err, "ログインしていなくてもエラーにしない")
	assert.True(t, revoked)
}

func TestSignOutClearsLocallyEvenWhenRevocationFails(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)
	e.F.Mu.Lock()
	e.F.RevokeStatus = http.StatusBadGateway
	e.F.Mu.Unlock()
	revoked, err := e.M.SignOut(t.Context(), siwctest.IssuedClient)
	require.NoError(t, err)
	assert.False(t, revoked, "失効を確認できなかったことを伝える")
	assert.Len(t, e.F.RevokeFormsCopy(), 3, "5xx は再試行する")
	_, serr := e.Sec.Get(siwctest.TokenKey("refresh"))
	assert.ErrorIs(t, serr, secrets.ErrNotFound)
}
