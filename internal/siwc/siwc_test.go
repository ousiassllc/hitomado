package siwc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/store"
)

var uuidURN = regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type savedReg struct {
	HostID        string `json:"hostId"`
	ClientID      string `json:"clientId"`
	Subject       string `json:"subject"`
	Email         string `json:"email"`
	ConsentNeeded bool   `json:"consentNeeded"`
	ExpiresAt     int64  `json:"expiresAt"`
}

func (e *env) reg(t *testing.T) savedReg {
	t.Helper()
	v, ok, _ := e.settings.GetSetting(t.Context(), store.SettingAIChatGPT)
	require.True(t, ok)
	var r savedReg
	require.NoError(t, json.Unmarshal([]byte(v), &r))
	return r
}

func TestSignInRegistersWithTheDocumentedParameters(t *testing.T) {
	e := newEnv(t, nil, nil)
	st := e.signIn(t)
	assert.Equal(t, siwc.Status{SignedIn: true, Email: "me@chatgpt.example"}, st)

	q := e.f.lastAuth()
	assert.Equal(t, "dynamic_agent_client", q.Get("client_id"))
	assert.Equal(t, "Hitomado", q.Get("agent_name_hint"))
	assert.Regexp(t, uuidURN, q.Get("ext_agent_host_id"))
	assert.Equal(t, "code", q.Get("response_type"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.NotEmpty(t, q.Get("code_challenge"))
	assert.NotEmpty(t, q.Get("state"))
	assert.NotEmpty(t, q.Get("nonce"))
	assert.Equal(t, "https://api.openai.com/v1", q.Get("resource"))
	assert.ElementsMatch(t,
		strings.Fields("openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"),
		strings.Fields(q.Get("scope")))
	assert.Regexp(t, `^http://127\.0\.0\.1:\d+/auth/callback$`, q.Get("redirect_uri"))
	for _, k := range []string{"client_secret", "id_token_hint", "login_hint", "prompt"} {
		assert.False(t, q.Has(k), "初回の登録に %s を付けない", k)
	}

	forms := e.f.tokenFormsCopy()
	require.Len(t, forms, 1)
	assert.Equal(t, "authorization_code", forms[0].Get("grant_type"))
	assert.Equal(t, issuedClient, forms[0].Get("client_id"), "交換には発行された client_id を使う")
	assert.Equal(t, q.Get("redirect_uri"), forms[0].Get("redirect_uri"))
	assert.Equal(t, "https://api.openai.com/v1", forms[0].Get("resource"))
	assert.NotEmpty(t, forms[0].Get("code_verifier"))
	assert.False(t, forms[0].Has("client_secret"))

	reg := e.reg(t)
	assert.Equal(t, issuedClient, reg.ClientID)
	assert.Equal(t, q.Get("ext_agent_host_id"), reg.HostID)
	assert.Equal(t, subjectA, reg.Subject)

	// トークンは資格情報ストアにだけ入る（登録情報には入らない）。
	v, _, _ := e.settings.GetSetting(t.Context(), store.SettingAIChatGPT)
	assert.NotContains(t, v, secretAccess)
	assert.NotContains(t, v, secretRefresh)
	got, err := e.sec.Get(secrets.ChatGPTTokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, secretRefresh, got)

	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok)
}

func TestReturningSignInReusesClientAndHost(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	first := e.f.lastAuth()

	_, err := e.m.SignOut(t.Context())
	require.NoError(t, err)
	// ログアウト後は ID トークンのヒントを送らない（docs: Omit it after signing out）。
	e.signIn(t)
	q := e.f.lastAuth()
	assert.Equal(t, issuedClient, q.Get("client_id"), "発行済みの client_id を再利用する")
	assert.False(t, q.Has("agent_name_hint"), "再認可には agent_name_hint を付けない")
	assert.False(t, q.Has("id_token_hint"))
	assert.Equal(t, "me@chatgpt.example", q.Get("login_hint"))
	assert.Equal(t, first.Get("ext_agent_host_id"), q.Get("ext_agent_host_id"), "同じホスト ID")

	// ログアウトせずに再度ログインする（期限切れなど）と、保存済みの ID トークンがヒントになる。
	e.signIn(t)
	q = e.f.lastAuth()
	assert.NotEmpty(t, q.Get("id_token_hint"))
	assert.Equal(t, issuedClient, q.Get("client_id"))
}

func TestDifferentAccountRegistersAnewAndRevokesTheOldSession(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	e.f.mu.Lock()
	e.f.subject = "sub-account-b"
	e.f.mu.Unlock()

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	assert.ErrorIs(t, err, siwc.ErrAccountMismatch, "保存済みの登録のまま別アカウントでは保存しない")
	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok, "失敗してもログイン中のセッションは変わらない")

	e.f.mu.Lock()
	e.f.accessToken = "ACCESS-SECRET-VALUE-B"
	e.f.mu.Unlock()
	_, err = e.m.SignIn(t.Context(), siwc.SignInOptions{DifferentAccount: true})
	require.NoError(t, err)
	q := e.f.lastAuth()
	assert.Equal(t, "dynamic_agent_client", q.Get("client_id"))
	assert.Equal(t, "Hitomado", q.Get("agent_name_hint"))
	assert.False(t, q.Has("id_token_hint"))
	assert.Equal(t, "sub-account-b", e.reg(t).Subject)
	tok, err = e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "ACCESS-SECRET-VALUE-B", tok)
}

func TestSignInDenied(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.callbackExtras = url.Values{"error": {"access_denied"}, "code": {""}}
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrDenied)
	assert.Empty(t, e.f.tokenFormsCopy(), "拒否されたらコードを交換しない")
	st, err := e.m.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, st.SignedIn)
}

func TestCallbackWithWrongStateIsIgnored(t *testing.T) {
	e := newEnv(t, nil, func(o *siwc.Options) {
		inner := o.OpenBrowser
		o.OpenBrowser = func(raw string) error {
			u, _ := url.Parse(raw)
			// 偽のコールバックが先に届いても無視され、正しいコールバックで完了する。
			resp, err := http.Get(u.Query().Get("redirect_uri") + "?code=evil&state=wrong&client_id=" + issuedClient)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			return inner(raw)
		}
	})
	st := e.signIn(t)
	assert.True(t, st.SignedIn)
	for _, f := range e.f.tokenFormsCopy() {
		assert.NotEqual(t, "evil", f.Get("code"))
	}
}

func TestPlanNotGrantedStoresNoTokensAndAsksConsentNextTime(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.scope = "email openid profile offline_access"
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrPlanNotGranted)
	_, err = e.sec.Get(secrets.ChatGPTTokenKey("access"))
	assert.ErrorIs(t, err, secrets.ErrNotFound)
	_, err = e.sec.Get(secrets.ChatGPTTokenKey("refresh"))
	assert.ErrorIs(t, err, secrets.ErrNotFound)
	assert.Len(t, e.f.revokeFormsCopy(), 1, "使わないリフレッシュトークンは失効させる")
	st, _ := e.m.Status(t.Context())
	assert.False(t, st.SignedIn)

	e.f.mu.Lock()
	e.f.scope = scopeAll
	e.f.mu.Unlock()
	e.signIn(t)
	q := e.f.lastAuth()
	assert.Equal(t, issuedClient, q.Get("client_id"))
	assert.Equal(t, "consent", q.Get("prompt"))
	assert.False(t, e.reg(t).ConsentNeeded)
	e.signIn(t)
	assert.False(t, e.f.lastAuth().Has("prompt"), "普通のログインでは同意を強制しない")
}

func TestInvalidIDTokenIsRejectedAndNothingIsSaved(t *testing.T) {
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tests := []struct {
		name   string
		mutate func(*fakeOpenAI)
	}{
		{"nonce が違う", func(f *fakeOpenAI) { f.idTokenNonce = "other-nonce" }},
		{"aud が違う", func(f *fakeOpenAI) { f.idTokenAud = "oaiapp_OTHER" }},
		{"署名鍵が違う", func(f *fakeOpenAI) { f.idTokenSigner = otherKey }},
		{"sub が無い", func(f *fakeOpenAI) { f.subject = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, nil, nil)
			tc.mutate(e.f)
			_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
			require.ErrorIs(t, err, siwc.ErrInvalidResponse)
			_, serr := e.sec.Get(secrets.ChatGPTTokenKey("access"))
			assert.ErrorIs(t, serr, secrets.ErrNotFound)
			assert.Empty(t, e.reg(t).ClientID, "検証に通るまで登録を保存しない")
		})
	}
}

func TestNewRegistrationWithoutIssuedClientIDIsIncomplete(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.omitClientID = true
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrInvalidResponse)
	assert.Empty(t, e.f.tokenFormsCopy(), "client_id が無ければ交換しない")
}

func TestReauthorizationRejectsADifferentClientID(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	e.f.callbackExtras = url.Values{"client_id": {"oaiapp_ATTACKER"}}
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrInvalidResponse)
	assert.Equal(t, issuedClient, e.reg(t).ClientID)
}

func TestSignInTimeoutAndCancel(t *testing.T) {
	silent := func(o *siwc.Options) { o.OpenBrowser = func(string) error { return nil } }

	t.Run("タイムアウト", func(t *testing.T) {
		e := newEnv(t, nil, func(o *siwc.Options) { silent(o); o.Timeout = 50 * time.Millisecond })
		_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
		assert.ErrorIs(t, err, siwc.ErrTimeout)
	})
	t.Run("CancelSignIn", func(t *testing.T) {
		opened := make(chan struct{})
		e := newEnv(t, nil, func(o *siwc.Options) {
			o.OpenBrowser = func(string) error { close(opened); return nil }
		})
		done := make(chan error, 1)
		go func() {
			_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
			done <- err
		}()
		<-opened
		_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
		assert.ErrorIs(t, err, siwc.ErrBusy, "同時に 2 つはログインしない")
		e.m.CancelSignIn()
		assert.ErrorIs(t, <-done, siwc.ErrCancelled)
	})
	t.Run("ctx キャンセル", func(t *testing.T) {
		e := newEnv(t, nil, silent)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := e.m.SignIn(ctx, siwc.SignInOptions{})
		assert.ErrorIs(t, err, siwc.ErrCancelled)
	})
}

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

func TestLongTokensAreSplitUnderTheCredentialStoreLimit(t *testing.T) {
	sec := limitedSecrets{Memory: secrets.NewMemory(), max: 2560}
	e := newEnv(t, sec, nil)
	long := strings.Repeat("A", 6500)
	e.f.accessToken = long
	e.signIn(t)

	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, long, tok)

	// 短い値へ更新したとき、古い断片が残らず、読み出しが正しい。
	e.f.mu.Lock()
	e.f.accessToken = "short"
	e.f.mu.Unlock()
	e.clk.Advance(2 * time.Hour)
	tok, err = e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "short-refreshed-1", tok)
	_, err = sec.Get(secrets.ChatGPTTokenKey("access") + "#2")
	assert.ErrorIs(t, err, secrets.ErrNotFound)

	_, err = e.m.SignOut(t.Context())
	require.NoError(t, err)
	_, err = sec.Get(secrets.ChatGPTTokenKey("access"))
	assert.ErrorIs(t, err, secrets.ErrNotFound)
}

func TestOversizedIDTokenDoesNotFailSignIn(t *testing.T) {
	e := newEnv(t, failingIDSecrets{secrets.NewMemory()}, nil)
	st := e.signIn(t)
	assert.True(t, st.SignedIn, "ID トークンは任意（ヒント用）なので保存できなくてもログインは成功する")
}

// failingIDSecrets は ID トークンの保存だけ失敗する資格情報ストア。
type failingIDSecrets struct{ *secrets.Memory }

func (f failingIDSecrets) Set(key, value string) error {
	if key == secrets.ChatGPTTokenKey("id") || strings.HasPrefix(key, secrets.ChatGPTTokenKey("id")+"#") {
		return errors.New("store refuses")
	}
	return f.Memory.Set(key, value)
}

// 秘密（トークン・認可コード・リフレッシュトークン）が、ログ・エラー文面・戻り値に出ないこと。
func TestSecretsNeverAppearInLogsErrorsOrStatus(t *testing.T) {
	// サーバーが失敗の応答に、秘密・要求の値・説明文をそのまま入れてくる最悪ケース。
	hostile := func(code string) string {
		return fmt.Sprintf(`{"error":%q,"error_description":"bad %s %s code-1","detail":"%s","access_token":"%s","refresh_token":"%s"}`, code,
			secretAccess, secretRefresh, secretAccess, secretAccess, secretRefresh)
	}
	forbidden := []string{secretAccess, secretRefresh, "code-1", "code_verifier"}

	var all []string
	collect := func(label string, err error) {
		if err != nil {
			all = append(all, label+": "+err.Error(), label+" %+v: "+fmt.Sprintf("%+v", err), label+" %#v: "+fmt.Sprintf("%#v", err))
		}
	}

	e := newEnv(t, nil, nil)
	e.signIn(t)
	// 更新が敵対的な応答を返す。
	e.clk.Advance(2 * time.Hour)
	e.f.mu.Lock()
	e.f.refreshStatus, e.f.refreshBody = http.StatusInternalServerError, hostile("invalid_request")
	e.f.mu.Unlock()
	_, err := e.m.AccessToken(t.Context())
	require.Error(t, err)
	collect("refresh 500", err)
	e.f.mu.Lock()
	e.f.refreshStatus, e.f.refreshBody = http.StatusBadRequest, hostile("invalid_grant")
	e.f.mu.Unlock()
	_, err = e.m.AccessToken(t.Context())
	require.ErrorIs(t, err, siwc.ErrReauthRequired)
	collect("refresh 400", err)

	// コード交換が敵対的な応答を返す。
	e2 := newEnv(t, nil, nil)
	e2.f.tokenStatus, e2.f.tokenBody = http.StatusBadRequest, hostile("invalid_request")
	_, err = e2.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.Error(t, err)
	collect("exchange 400", err)
	e2.f.mu.Lock()
	e2.f.tokenStatus, e2.f.tokenBody = http.StatusInternalServerError, hostile("invalid_request")
	e2.f.mu.Unlock()
	_, err = e2.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.Error(t, err)
	collect("exchange 500", err)

	// 検証に失敗する ID トークン。
	e3 := newEnv(t, nil, nil)
	e3.f.idTokenNonce = "x"
	_, err = e3.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.Error(t, err)
	collect("bad id token", err)

	// 失効が敵対的な応答を返しても、ログアウトは成功し、本文はログにも出ない。
	e4 := newEnv(t, nil, nil)
	e4.signIn(t)
	e4.f.mu.Lock()
	e4.f.revokeStatus = http.StatusInternalServerError
	e4.f.mu.Unlock()
	_, err = e4.m.SignOut(t.Context())
	require.NoError(t, err)

	for _, s := range all {
		for _, bad := range forbidden {
			assert.NotContains(t, s, bad)
		}
	}
	for _, x := range []*env{e, e2, e3, e4} {
		logs := x.logs.String()
		for _, bad := range forbidden {
			assert.NotContains(t, logs, bad)
		}
		assert.NotContains(t, logs, "id_token")
		assert.NotContains(t, logs, "eyJ", "JWT の形のものが出ない")
	}
	// Status はトークンを持たない型で、表示用のメールだけを返す。
	st, err := e.m.Status(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, fmt.Sprintf("%+v", st), secretAccess)
}
