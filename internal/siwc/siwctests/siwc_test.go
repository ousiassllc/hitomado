package siwctests

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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
