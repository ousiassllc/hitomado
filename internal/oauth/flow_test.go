package oauth_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/oauth"
)

var cfg = oauth.ClientConfig{ClientID: testClientID, ClientSecret: testClientSecret}

func okCallback(t *testing.T, p authParams) {
	redirectWith(t, p, url.Values{"code": {testCode}, "state": {p.State}})
}

func TestAuthorizeSuccess(t *testing.T) {
	g := newFakeGoogle(t)
	b := &browser{g: g, t: t, behave: okCallback}

	res, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.NoError(t, err)

	assert.Equal(t, "taro.yamada@example.com", res.Email)
	assert.Equal(t, "山田 太郎", res.Name)
	assert.Equal(t, testRefresh, res.RefreshToken)

	q := b.params.Query
	assert.Equal(t, testClientID, q.Get("client_id"))
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "offline", q.Get("access_type"))
	assert.Equal(t, "consent", q.Get("prompt"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.Equal(t, strings.Join(oauth.Scopes, " "), q.Get("scope"))
	assert.GreaterOrEqual(t, len(q.Get("state")), 32)
	assert.GreaterOrEqual(t, len(q.Get("code_challenge")), 43)
	assert.True(t, strings.HasPrefix(b.params.Redirect, "http://127.0.0.1:"), b.params.Redirect)

	g.mu.Lock()
	require.Len(t, g.tokenForms, 1)
	assert.Equal(t, b.params.Redirect, g.tokenForms[0].Get("redirect_uri"))
	g.mu.Unlock()

	assertListenerClosed(t, b.params.Redirect)
}

func TestAuthorizeCallbackPage(t *testing.T) {
	g := newFakeGoogle(t)
	var body string
	var status int
	b := &browser{g: g, t: t, behave: func(t *testing.T, p authParams) {
		status, body = callback(t, p.Redirect, url.Values{"code": {testCode}, "state": {p.State}})
	}}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, "Hitomado に戻って")
	assert.Contains(t, body, `lang="ja"`)
	for _, banned := range []string{"<script", "src=", "href=", "http://", "https://", "@import", "url("} {
		assert.NotContains(t, body, banned, "外部リソースを参照してはならない")
	}
	assert.NotContains(t, body, testCode)
}

func TestAuthorizeStateMismatchIsIgnored(t *testing.T) {
	g := newFakeGoogle(t)
	b := &browser{g: g, t: t, behave: func(t *testing.T, p authParams) {
		// 偽の state（攻撃者のコールバック）は 400 で無視され、待受は続く。
		status, _ := callback(t, p.Redirect, url.Values{"code": {"attacker-code"}, "state": {"wrong"}})
		assert.Equal(t, http.StatusBadRequest, status)
		status, _ = callback(t, p.Redirect, url.Values{"error": {"access_denied"}})
		assert.Equal(t, http.StatusBadRequest, status)
		okCallback(t, p)
	}}

	res, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, testRefresh, res.RefreshToken)

	g.mu.Lock()
	require.Len(t, g.tokenForms, 1, "不正なコールバックでコード交換してはならない")
	assert.Equal(t, testCode, g.tokenForms[0].Get("code"))
	g.mu.Unlock()
}

func TestAuthorizeAccessDenied(t *testing.T) {
	g := newFakeGoogle(t)
	var body string
	b := &browser{g: g, t: t, behave: func(t *testing.T, p authParams) {
		_, body = callback(t, p.Redirect, url.Values{"error": {"access_denied"}, "state": {p.State}})
	}}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.ErrorIs(t, err, oauth.ErrDenied)
	assert.Contains(t, body, "完了しませんでした")
	assert.Empty(t, g.tokenForms)
	assertListenerClosed(t, b.params.Redirect)
}

func TestAuthorizeOtherProviderError(t *testing.T) {
	g := newFakeGoogle(t)
	b := &browser{g: g, t: t, behave: func(t *testing.T, p authParams) {
		callback(t, p.Redirect, url.Values{"error": {"server_error"}, "state": {p.State}})
	}}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)
	assert.NotErrorIs(t, err, oauth.ErrDenied)
	assert.NotErrorIs(t, err, oauth.ErrTimeout)
}

func TestAuthorizeMissingScopeIsDenied(t *testing.T) {
	g := newFakeGoogle(t)
	g.scope = "openid email profile" // 利用者が gmail.modify のチェックを外した
	b := &browser{g: g, t: t, behave: okCallback}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.ErrorIs(t, err, oauth.ErrDenied)
	assertListenerClosed(t, b.params.Redirect)
}

func TestAuthorizeNoRefreshTokenIsDenied(t *testing.T) {
	g := newFakeGoogle(t)
	g.refreshToken = ""
	b := &browser{g: g, t: t, behave: okCallback}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.ErrorIs(t, err, oauth.ErrDenied)
}

func TestAuthorizeTokenEndpointFailureHidesSecrets(t *testing.T) {
	g := newFakeGoogle(t)
	g.tokenStatus = http.StatusBadRequest
	b := &browser{g: g, t: t, behave: okCallback}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)
	assert.NotErrorIs(t, err, oauth.ErrDenied)
	for _, secret := range []string{testRefresh, testCode, testClientSecret, b.params.State} {
		assert.NotContains(t, err.Error(), secret)
	}
}

func TestAuthorizeUserinfoFailure(t *testing.T) {
	g := newFakeGoogle(t)
	g.userinfoEmail = ""
	b := &browser{g: g, t: t, behave: okCallback}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testRefresh)
}

func TestAuthorizeTimeout(t *testing.T) {
	g := newFakeGoogle(t)
	b := &browser{g: g, t: t}

	_, err := g.flow(b.open, 50*time.Millisecond).Authorize(context.Background(), cfg)
	require.ErrorIs(t, err, oauth.ErrTimeout)
	assertListenerClosed(t, b.params.Redirect)
}

func TestAuthorizeCancelled(t *testing.T) {
	g := newFakeGoogle(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &browser{g: g, t: t, behave: func(*testing.T, authParams) { cancel() }}

	_, err := g.flow(b.open, time.Minute).Authorize(ctx, cfg)
	require.ErrorIs(t, err, oauth.ErrCancelled)
	assertListenerClosed(t, b.params.Redirect)
}

func TestAuthorizeOpenBrowserError(t *testing.T) {
	g := newFakeGoogle(t)
	boom := errors.New("no browser")
	b := &browser{g: g, t: t, openErr: boom}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.ErrorIs(t, err, boom)
}

func TestAuthorizeStateAndChallengeAreRandom(t *testing.T) {
	g := newFakeGoogle(t)
	seen := map[string]bool{}
	for range 3 {
		b := &browser{g: g, t: t, behave: okCallback}
		_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
		require.NoError(t, err)
		for _, v := range []string{b.params.State, b.params.Query.Get("code_challenge")} {
			require.False(t, seen[v], "乱数が重複した")
			seen[v] = true
		}
	}
}

func TestRevoke(t *testing.T) {
	g := newFakeGoogle(t)
	f := g.flow(nil, time.Minute)

	require.NoError(t, f.Revoke(context.Background(), testRefresh))
	g.mu.Lock()
	require.Len(t, g.revokeForms, 1)
	assert.Equal(t, testRefresh, g.revokeForms[0].Get("token"))
	g.revokeStatus = http.StatusBadRequest
	g.mu.Unlock()

	err := f.Revoke(context.Background(), testRefresh)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), testRefresh)
}

func TestExchangeIsNotBoundByWaitTimeout(t *testing.T) {
	g := newFakeGoogle(t)
	g.tokenDelay = 300 * time.Millisecond // 待受の Timeout（50ms）を超えてもコード受領後の交換は成功する
	b := &browser{g: g, t: t, behave: okCallback}

	f := oauth.NewFlow(oauth.Options{
		OpenBrowser: b.open,
		AuthURL:     g.srv.URL + "/auth",
		TokenURL:    g.srv.URL + "/token",
		UserinfoURL: g.srv.URL + "/userinfo",
		HTTPClient:  g.srv.Client(),
		Timeout:     50 * time.Millisecond,
	})
	res, err := f.Authorize(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, testRefresh, res.RefreshToken)
}

func TestExchangeTimeout(t *testing.T) {
	g := newFakeGoogle(t)
	g.tokenBlock = true
	b := &browser{g: g, t: t, behave: okCallback}

	f := oauth.NewFlow(oauth.Options{
		OpenBrowser:     b.open,
		AuthURL:         g.srv.URL + "/auth",
		TokenURL:        g.srv.URL + "/token",
		UserinfoURL:     g.srv.URL + "/userinfo",
		HTTPClient:      g.srv.Client(),
		Timeout:         time.Minute,
		ExchangeTimeout: 50 * time.Millisecond,
	})
	_, err := f.Authorize(context.Background(), cfg)
	require.ErrorIs(t, err, oauth.ErrTimeout)
}

func TestExchangeCancelled(t *testing.T) {
	g := newFakeGoogle(t)
	g.tokenBlock = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &browser{g: g, t: t, behave: func(t *testing.T, p authParams) {
		okCallback(t, p)
		cancel()
	}}

	_, err := g.flow(b.open, time.Minute).Authorize(ctx, cfg)
	require.ErrorIs(t, err, oauth.ErrCancelled)
}
