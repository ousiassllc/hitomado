package siwctests

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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
	"github.com/ousiassllc/hitomado/internal/siwc/siwctest"
	"github.com/ousiassllc/hitomado/internal/store"
)

var uuidURN = regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestSignInRegistersWithTheDocumentedParameters(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	st := e.SignIn(t)
	assert.Equal(t, siwc.Status{
		SignedIn: true, Email: "me@chatgpt.example",
		Accounts: []siwc.Account{{ID: siwctest.IssuedClient, Email: "me@chatgpt.example", SignedIn: true, Active: true, PlanNoticePending: true}},
	}, st)

	q := e.F.LastAuth()
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

	forms := e.F.TokenFormsCopy()
	require.Len(t, forms, 1)
	assert.Equal(t, "authorization_code", forms[0].Get("grant_type"))
	assert.Equal(t, siwctest.IssuedClient, forms[0].Get("client_id"), "交換には発行された client_id を使う")
	assert.Equal(t, q.Get("redirect_uri"), forms[0].Get("redirect_uri"))
	assert.Equal(t, "https://api.openai.com/v1", forms[0].Get("resource"))
	assert.NotEmpty(t, forms[0].Get("code_verifier"))
	assert.False(t, forms[0].Has("client_secret"))

	// トークンは資格情報ストアにだけ入る（登録情報には入らない）。
	v, _, _ := e.Settings.GetSetting(t.Context(), store.SettingAIChatGPT)
	assert.NotContains(t, v, siwctest.SecretAccess)
	assert.NotContains(t, v, siwctest.SecretRefresh)
	got, err := e.Sec.Get(siwctest.TokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretRefresh, got)

	tok, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess, tok)
}

func TestReturningSignInReusesClientAndHost(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)
	first := e.F.LastAuth()

	_, err := e.M.SignOut(t.Context(), siwctest.IssuedClient)
	require.NoError(t, err)
	// ログアウト後は ID トークンのヒントを送らない（docs: Omit it after signing out）。
	e.Relogin(t)
	q := e.F.LastAuth()
	assert.Equal(t, siwctest.IssuedClient, q.Get("client_id"), "発行済みの client_id を再利用する")
	assert.False(t, q.Has("agent_name_hint"), "再認可には agent_name_hint を付けない")
	assert.False(t, q.Has("id_token_hint"))
	assert.Equal(t, "me@chatgpt.example", q.Get("login_hint"))
	assert.Equal(t, first.Get("ext_agent_host_id"), q.Get("ext_agent_host_id"), "同じホスト ID")

	// ログアウトせずに再度ログインする（期限切れなど）と、保存済みの ID トークンがヒントになる。
	e.Relogin(t)
	q = e.F.LastAuth()
	assert.NotEmpty(t, q.Get("id_token_hint"))
	assert.Equal(t, siwctest.IssuedClient, q.Get("client_id"))
}

// 別の ChatGPT アカウントの追加は、ID トークンを検証して保存するまで、使用中のアカウントに影響しない。
// 追加が済んでも、もとのアカウントのトークンと登録は残る（置き換えない）。
func TestAddingAnotherAccountKeepsTheFirstOne(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)

	// 選んだ登録（A）のまま別アカウント（B）でログインした場合は保存しない。
	e.F.Mu.Lock()
	e.F.Subject = "sub-account-b"
	e.F.Mu.Unlock()
	_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{AccountID: siwctest.IssuedClient})
	assert.ErrorIs(t, err, siwc.ErrAccountMismatch, "選んだ登録のまま別アカウントでは保存しない")
	tok, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess, tok, "失敗してもログイン中のセッションは変わらない")
	assert.Len(t, e.Reg(t).Accounts, 1)
	revokesBefore := len(e.F.RevokeFormsCopy()) // 不一致で得た新しいセッションの失効（これは期待どおり）

	// 新規登録として追加する。
	e.F.Mu.Lock()
	e.F.AccessToken = "ACCESS-SECRET-VALUE-B"
	e.F.IssueQueue = []string{"oaiapp_SECOND"}
	e.F.Mu.Unlock()
	st, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.NoError(t, err)
	q := e.F.LastAuth()
	assert.Equal(t, "dynamic_agent_client", q.Get("client_id"))
	assert.Equal(t, "Hitomado", q.Get("agent_name_hint"))
	for _, k := range []string{"id_token_hint", "login_hint", "prompt"} {
		assert.False(t, q.Has(k), "追加の登録に別アカウントの %s を付けない", k)
	}
	assert.Equal(t, e.Reg(t).HostID, q.Get("ext_agent_host_id"), "ホスト ID は全アカウントで共通")
	assert.Equal(t, []siwc.Account{
		{ID: siwctest.IssuedClient, Email: "me@chatgpt.example", SignedIn: true, PlanNoticePending: true},
		{ID: "oaiapp_SECOND", Email: "me@chatgpt.example", SignedIn: true, Active: true, PlanNoticePending: true},
	}, st.Accounts, "メールが同じでも別の登録")
	assert.Equal(t, "oaiapp_SECOND", e.Reg(t).Active, "ログインを終えたアカウントが使用中になる")

	tok, err = e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "ACCESS-SECRET-VALUE-B", tok)
	assert.Len(t, e.F.RevokeFormsCopy(), revokesBefore, "追加しても、もとのアカウントのセッションは失効させない")
	got, err := e.Sec.Get(siwctest.TokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretRefresh, got, "もとのアカウントのトークンは残る")
}

func TestSignInDenied(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.F.CallbackExtras = url.Values{"error": {"access_denied"}, "code": {""}}
	_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrDenied)
	assert.Empty(t, e.F.TokenFormsCopy(), "拒否されたらコードを交換しない")
	st, err := e.M.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, st.SignedIn)
}

func TestCallbackWithWrongStateIsIgnored(t *testing.T) {
	e := siwctest.NewEnv(t, nil, func(o *siwc.Options) {
		inner := o.OpenBrowser
		o.OpenBrowser = func(raw string) error {
			u, _ := url.Parse(raw)
			// 偽のコールバックが先に届いても無視され、正しいコールバックで完了する。
			resp, err := http.Get(u.Query().Get("redirect_uri") + "?code=evil&state=wrong&client_id=" + siwctest.IssuedClient)
			require.NoError(t, err)
			_ = resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			return inner(raw)
		}
	})
	st := e.SignIn(t)
	assert.True(t, st.SignedIn)
	for _, f := range e.F.TokenFormsCopy() {
		assert.NotEqual(t, "evil", f.Get("code"))
	}
}

func TestPlanNotGrantedStoresNoTokensAndAsksConsentNextTime(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.F.Scope = "email openid profile offline_access"
	_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrPlanNotGranted)
	_, err = e.Sec.Get(siwctest.TokenKey("access"))
	assert.ErrorIs(t, err, secrets.ErrNotFound)
	_, err = e.Sec.Get(siwctest.TokenKey("refresh"))
	assert.ErrorIs(t, err, secrets.ErrNotFound)
	assert.Len(t, e.F.RevokeFormsCopy(), 1, "使わないリフレッシュトークンは失効させる")
	st, _ := e.M.Status(t.Context())
	assert.False(t, st.SignedIn)

	assert.True(t, e.Reg(t).First().ConsentNeeded, "ログイン中でない登録は、次のログインのために残す")
	assert.Empty(t, e.Reg(t).Active, "使用中にはしない")
	e.F.Mu.Lock()
	e.F.Scope = siwctest.ScopeAll
	e.F.Mu.Unlock()
	e.Relogin(t)
	q := e.F.LastAuth()
	assert.Equal(t, siwctest.IssuedClient, q.Get("client_id"))
	assert.Equal(t, "consent", q.Get("prompt"))
	assert.False(t, e.Reg(t).First().ConsentNeeded)
	e.Relogin(t)
	assert.False(t, e.F.LastAuth().Has("prompt"), "普通のログインでは同意を強制しない")
}

func TestInvalidIDTokenIsRejectedAndNothingIsSaved(t *testing.T) {
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tests := []struct {
		name   string
		mutate func(*siwctest.FakeOpenAI)
	}{
		{"nonce が違う", func(f *siwctest.FakeOpenAI) { f.IDTokenNonce = "other-nonce" }},
		{"aud が違う", func(f *siwctest.FakeOpenAI) { f.IDTokenAud = "oaiapp_OTHER" }},
		{"署名鍵が違う", func(f *siwctest.FakeOpenAI) { f.IDTokenSigner = otherKey }},
		{"sub が無い", func(f *siwctest.FakeOpenAI) { f.Subject = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := siwctest.NewEnv(t, nil, nil)
			tc.mutate(e.F)
			_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
			require.ErrorIs(t, err, siwc.ErrInvalidResponse)
			_, serr := e.Sec.Get(siwctest.TokenKey("access"))
			assert.ErrorIs(t, serr, secrets.ErrNotFound)
			assert.Empty(t, e.Reg(t).Accounts, "検証に通るまで登録を保存しない")
		})
	}
}

func TestNewRegistrationWithoutIssuedClientIDIsIncomplete(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.F.OmitClientID = true
	_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrInvalidResponse)
	assert.Empty(t, e.F.TokenFormsCopy(), "client_id が無ければ交換しない")
}

// 再認可で別の client_id（大文字小文字だけ違う値も。Windows の資格情報ストアでは同じキー）が返ったら、コードを交換する前に拒否する。
func TestReauthorizationRejectsADifferentClientID(t *testing.T) {
	for _, other := range []string{"oaiapp_ATTACKER", strings.ToLower(siwctest.IssuedClient)} {
		e := siwctest.NewEnv(t, secrets.FoldKeyCase(secrets.NewMemory()), nil)
		e.SignIn(t)
		forms := len(e.F.TokenFormsCopy())
		e.F.CallbackExtras = url.Values{"client_id": {other}}
		_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{AccountID: siwctest.IssuedClient})
		require.ErrorIs(t, err, siwc.ErrInvalidResponse)
		assert.Equal(t, siwctest.IssuedClient, e.Reg(t).First().ClientID)
		assert.Len(t, e.F.TokenFormsCopy(), forms, "コードを交換しない")
	}
}

func TestSignInTimeoutAndCancel(t *testing.T) {
	silent := func(o *siwc.Options) { o.OpenBrowser = func(string) error { return nil } }

	t.Run("タイムアウト", func(t *testing.T) {
		e := siwctest.NewEnv(t, nil, func(o *siwc.Options) { silent(o); o.Timeout = 50 * time.Millisecond })
		_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
		assert.ErrorIs(t, err, siwc.ErrTimeout)
	})
	t.Run("CancelSignIn", func(t *testing.T) {
		opened := make(chan struct{})
		e := siwctest.NewEnv(t, nil, func(o *siwc.Options) {
			o.OpenBrowser = func(string) error { close(opened); return nil }
		})
		done := make(chan error, 1)
		go func() {
			_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
			done <- err
		}()
		<-opened
		_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
		assert.ErrorIs(t, err, siwc.ErrBusy, "同時に 2 つはログインしない")
		e.M.CancelSignIn()
		assert.ErrorIs(t, <-done, siwc.ErrCancelled)
	})
	t.Run("ctx キャンセル", func(t *testing.T) {
		e := siwctest.NewEnv(t, nil, silent)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := e.M.SignIn(ctx, siwc.SignInOptions{})
		assert.ErrorIs(t, err, siwc.ErrCancelled)
	})
}
