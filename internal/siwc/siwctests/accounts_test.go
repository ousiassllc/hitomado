package siwctests

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

const (
	secondClient = "oaiapp_SECOND"
	secondAccess = "ACCESS-SECRET-VALUE-2" //nolint:gosec // テスト用のダミー値
)

func secondTokenKey(kind string) string { return secrets.ChatGPTTokenKey(secondClient, kind) }

// twoAccounts は同じメールアドレスの 2 つの登録（issuedClient と secondClient）にログインした状態にする。
// 2 つ目のログインのあと、使用中は secondClient。
func twoAccounts(t *testing.T, sec secrets.Store, mutate func(*siwc.Options)) *env {
	t.Helper()
	e := newEnv(t, sec, mutate)
	e.signIn(t)
	e.f.mu.Lock()
	e.f.accessToken = secondAccess
	e.f.subject = "sub-account-b"
	e.f.issueQueue = []string{secondClient}
	e.f.mu.Unlock()
	st, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.NoError(t, err)
	require.Len(t, st.Accounts, 2)
	return e
}

func TestAccountsKeepSeparateTokensAndRegistrations(t *testing.T) {
	e := twoAccounts(t, nil, nil)

	a, err := e.sec.Get(tokenKey("access"))
	require.NoError(t, err)
	assert.Equal(t, secretAccess, a)
	b, err := e.sec.Get(secondTokenKey("access"))
	require.NoError(t, err)
	assert.Equal(t, secondAccess, b)
	ra, err := e.sec.Get(tokenKey("refresh"))
	require.NoError(t, err)
	rb, err := e.sec.Get(secondTokenKey("refresh"))
	require.NoError(t, err)
	assert.NotEqual(t, ra, rb, "リフレッシュトークンも別")

	reg := e.reg(t)
	require.Len(t, reg.Accounts, 2)
	assert.Equal(t, savedAcct{ClientID: issuedClient, Subject: subjectA, Email: "me@chatgpt.example", ExpiresAt: reg.Accounts[0].ExpiresAt, PlanNotice: "pending"}, reg.Accounts[0])
	assert.Equal(t, "sub-account-b", reg.Accounts[1].Subject)
	assert.Equal(t, reg.Accounts[0].Email, reg.Accounts[1].Email, "同じメールでも別の登録")
	assert.Equal(t, secondClient, reg.Active)
}

func TestSelectAccountSwitchesTheToken(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secondAccess, tok)

	require.NoError(t, e.m.SelectAccount(t.Context(), issuedClient))
	tok, err = e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok, "切り替えたアカウントのトークンを返す")
	st, err := e.m.Status(t.Context())
	require.NoError(t, err)
	assert.True(t, st.SignedIn)
	assert.True(t, st.Accounts[0].Active)
	assert.False(t, st.Accounts[1].Active)
	assert.Len(t, e.f.tokenFormsCopy(), 2, "切り替えだけでは通信しない（ログイン 2 回分のみ）")

	assert.ErrorIs(t, e.m.SelectAccount(t.Context(), "oaiapp_UNKNOWN"), siwc.ErrUnknownAccount)
	assert.Equal(t, issuedClient, e.reg(t).Active, "失敗したら選択は変わらない")
}

func TestSelectAccountRejectsASignedOutAccount(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	_, err := e.m.SignOut(t.Context(), issuedClient)
	require.NoError(t, err)
	assert.ErrorIs(t, e.m.SelectAccount(t.Context(), issuedClient), siwc.ErrNotSignedIn)
	assert.Equal(t, secondClient, e.reg(t).Active)
}

// 片方をサインアウトしても、もう片方のトークンと登録は残る。失効するのも選んだアカウントのトークンだけ。
func TestSignOutAffectsOnlyTheSelectedAccount(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	require.NoError(t, e.m.SelectAccount(t.Context(), issuedClient))

	revoked, err := e.m.SignOut(t.Context(), secondClient) // 使用中でない方をログアウト
	require.NoError(t, err)
	assert.True(t, revoked)
	forms := e.f.revokeFormsCopy()
	require.Len(t, forms, 1)
	assert.Equal(t, secondClient, forms[0].Get("client_id"))
	assert.Equal(t, refreshTokenFor(secondClient), forms[0].Get("token"), "選んだアカウントのリフレッシュトークンだけを失効させる")

	for _, k := range []string{"access", "refresh", "id"} {
		_, err := e.sec.Get(secondTokenKey(k))
		assert.ErrorIs(t, err, secrets.ErrNotFound, "ログアウトしたアカウントの %s は消える", k)
	}
	got, err := e.sec.Get(tokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, secretRefresh, got, "もう片方のトークンは残る")
	_, err = e.sec.Get(tokenKey("id"))
	require.NoError(t, err)

	reg := e.reg(t)
	require.Len(t, reg.Accounts, 2, "どちらの登録も残る")
	assert.Equal(t, issuedClient, reg.Active, "使用中のアカウントは変わらない")
	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok)
	st, err := e.m.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []siwc.Account{
		{ID: issuedClient, Email: "me@chatgpt.example", SignedIn: true, Active: true, PlanNoticePending: true},
		{ID: secondClient, Email: "me@chatgpt.example", PlanNoticePending: true},
	}, st.Accounts)
}

func TestSigningOutTheActiveAccountLeavesNoneActiveAndKeepsTheOther(t *testing.T) {
	e := twoAccounts(t, nil, nil) // 使用中は secondClient
	_, err := e.m.SignOut(t.Context(), secondClient)
	require.NoError(t, err)

	st, err := e.m.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, st.SignedIn, "使用中のアカウントがログアウトしたら、未ログイン（ほかのアカウントへ黙って切り替えない）")
	_, err = e.m.AccessToken(t.Context())
	assert.ErrorIs(t, err, siwc.ErrNotSignedIn)

	require.NoError(t, e.m.SelectAccount(t.Context(), issuedClient), "残したアカウントは選び直せる")
	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok)
}

func TestSignOutOfAnUnknownAccountChangesNothing(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	_, err := e.m.SignOut(t.Context(), "oaiapp_UNKNOWN")
	assert.ErrorIs(t, err, siwc.ErrUnknownAccount)
	_, err = e.m.SignOut(t.Context(), "")
	assert.ErrorIs(t, err, siwc.ErrUnknownAccount, "アカウントの指定は必須（使用中を暗黙に消さない）")
	assert.Empty(t, e.f.revokeFormsCopy())
	st, _ := e.m.Status(t.Context())
	assert.True(t, st.Accounts[0].SignedIn)
	assert.True(t, st.Accounts[1].SignedIn)
}

// 更新は使用中のアカウントの client_id・リフレッシュトークンで行い、失敗で消えるのもそのアカウントのトークンだけ。
func TestRefreshUsesAndAffectsOnlyTheActiveAccount(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	e.clk.Advance(2 * time.Hour)

	_, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	forms := e.f.tokenFormsCopy()
	last := forms[len(forms)-1]
	assert.Equal(t, "refresh_token", last.Get("grant_type"))
	assert.Equal(t, secondClient, last.Get("client_id"))
	assert.Equal(t, refreshTokenFor(secondClient), last.Get("refresh_token"))
	got, err := e.sec.Get(tokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, secretRefresh, got, "使用中でないアカウントのトークンは更新で変わらない")

	e.f.mu.Lock()
	e.f.refreshStatus, e.f.refreshBody = http.StatusBadRequest, `{"error":"invalid_grant"}`
	e.f.mu.Unlock()
	e.clk.Advance(2 * time.Hour)
	_, err = e.m.AccessToken(t.Context())
	require.ErrorIs(t, err, siwc.ErrReauthRequired)
	_, err = e.sec.Get(secondTokenKey("refresh"))
	assert.ErrorIs(t, err, secrets.ErrNotFound, "失敗したアカウントのトークンは消す")
	got, err = e.sec.Get(tokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, secretRefresh, got, "もう片方のトークンは残る")
	assert.Len(t, e.reg(t).Accounts, 2)

	// 選び直せば、もう片方で更新できる（更新は選んだアカウントの client_id）。
	e.f.mu.Lock()
	e.f.refreshStatus = 0
	e.f.mu.Unlock()
	require.NoError(t, e.m.SelectAccount(t.Context(), issuedClient))
	_, err = e.m.AccessToken(t.Context())
	require.NoError(t, err)
	forms = e.f.tokenFormsCopy()
	assert.Equal(t, issuedClient, forms[len(forms)-1].Get("client_id"))
}

// 再ログインは選んだ登録の client_id・ID トークン・メールだけを使い、ほかのアカウントのものを送らない。
func TestReloginUsesOnlyTheSelectedAccountsHints(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	idA, err := e.sec.Get(tokenKey("id"))
	require.NoError(t, err)
	idB, err := e.sec.Get(secondTokenKey("id"))
	require.NoError(t, err)
	require.NotEqual(t, idA, idB)

	e.f.mu.Lock()
	e.f.subject = subjectA
	e.f.mu.Unlock()
	st, err := e.m.SignIn(t.Context(), siwc.SignInOptions{AccountID: issuedClient})
	require.NoError(t, err)
	q := e.f.lastAuth()
	assert.Equal(t, issuedClient, q.Get("client_id"))
	assert.False(t, q.Has("agent_name_hint"))
	assert.NotEmpty(t, q.Get("id_token_hint"))
	assert.NotEqual(t, idB, q.Get("id_token_hint"), "ほかのアカウントの ID トークンは送らない")
	assert.Equal(t, "me@chatgpt.example", q.Get("login_hint"))
	assert.Equal(t, issuedClient, e.reg(t).Active, "ログインを終えたアカウントが使用中になる")
	assert.True(t, st.SignedIn)

	// 別のアカウントの登録が選ばれたまま、メールだけが同じ別人でログインされても、sub が違えば保存しない。
	e.f.mu.Lock()
	e.f.subject = "sub-someone-else"
	e.f.mu.Unlock()
	_, err = e.m.SignIn(t.Context(), siwc.SignInOptions{AccountID: secondClient})
	assert.ErrorIs(t, err, siwc.ErrAccountMismatch)
	assert.Equal(t, "sub-account-b", e.reg(t).Accounts[1].Subject)

	_, err = e.m.SignIn(t.Context(), siwc.SignInOptions{AccountID: "oaiapp_UNKNOWN"})
	assert.ErrorIs(t, err, siwc.ErrUnknownAccount)
}

// 別のアカウントの追加で ChatGPT プランが許可されなかったとき、使用中のアカウントとそのトークンは変えない。
func TestPlanDeclinedWhileAddingAnAccountKeepsTheActiveSession(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	e.f.mu.Lock()
	e.f.scope = "email openid profile offline_access"
	e.f.subject = "sub-account-b"
	e.f.issueQueue = []string{secondClient}
	e.f.mu.Unlock()

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrPlanNotGranted)

	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok)
	reg := e.reg(t)
	assert.Equal(t, issuedClient, reg.Active)
	require.Len(t, reg.Accounts, 2)
	assert.True(t, reg.Accounts[1].ConsentNeeded, "追加しようとした登録は、次のログインで同意を求め直すために残す")
	assert.False(t, reg.Accounts[0].ConsentNeeded)
	_, err = e.sec.Get(secondTokenKey("refresh"))
	assert.ErrorIs(t, err, secrets.ErrNotFound, "使わないトークンは保存しない")
	st, _ := e.m.Status(t.Context())
	assert.False(t, st.Accounts[1].SignedIn)
}
