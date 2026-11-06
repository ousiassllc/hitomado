package siwctests

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/siwc/siwctest"
)

const (
	secondClient = "oaiapp_SECOND"
	secondAccess = "ACCESS-SECRET-VALUE-2" //nolint:gosec // テスト用のダミー値
)

func secondTokenKey(kind string) string { return secrets.ChatGPTTokenKey(secondClient, kind) }

// twoAccounts は同じメールアドレスの 2 つの登録（siwctest.IssuedClient と secondClient）にログインした状態にする。
// 2 つ目のログインのあと、使用中は secondClient。
func twoAccounts(t *testing.T, sec secrets.Store, mutate func(*siwc.Options)) *siwctest.Env {
	t.Helper()
	e := siwctest.NewEnv(t, sec, mutate)
	e.SignIn(t)
	e.F.Mu.Lock()
	e.F.AccessToken = secondAccess
	e.F.Subject = "sub-account-b"
	e.F.IssueQueue = []string{secondClient}
	e.F.Mu.Unlock()
	st, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.NoError(t, err)
	require.Len(t, st.Accounts, 2)
	return e
}

func TestAccountsKeepSeparateTokensAndRegistrations(t *testing.T) {
	e := twoAccounts(t, nil, nil)

	a, err := e.Sec.Get(siwctest.TokenKey("access"))
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess, a)
	b, err := e.Sec.Get(secondTokenKey("access"))
	require.NoError(t, err)
	assert.Equal(t, secondAccess, b)
	ra, err := e.Sec.Get(siwctest.TokenKey("refresh"))
	require.NoError(t, err)
	rb, err := e.Sec.Get(secondTokenKey("refresh"))
	require.NoError(t, err)
	assert.NotEqual(t, ra, rb, "リフレッシュトークンも別")

	reg := e.Reg(t)
	require.Len(t, reg.Accounts, 2)
	assert.Equal(t, siwctest.SavedAcct{ClientID: siwctest.IssuedClient, Subject: siwctest.SubjectA, Email: "me@chatgpt.example", ExpiresAt: reg.Accounts[0].ExpiresAt, PlanNotice: "pending"}, reg.Accounts[0])
	assert.Equal(t, "sub-account-b", reg.Accounts[1].Subject)
	assert.Equal(t, reg.Accounts[0].Email, reg.Accounts[1].Email, "同じメールでも別の登録")
	assert.Equal(t, secondClient, reg.Active)
}

func TestSelectAccountSwitchesTheToken(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	tok, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secondAccess, tok)

	require.NoError(t, e.M.SelectAccount(t.Context(), siwctest.IssuedClient))
	tok, err = e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess, tok, "切り替えたアカウントのトークンを返す")
	st, err := e.M.Status(t.Context())
	require.NoError(t, err)
	assert.True(t, st.SignedIn)
	assert.True(t, st.Accounts[0].Active)
	assert.False(t, st.Accounts[1].Active)
	assert.Len(t, e.F.TokenFormsCopy(), 2, "切り替えだけでは通信しない（ログイン 2 回分のみ）")

	assert.ErrorIs(t, e.M.SelectAccount(t.Context(), "oaiapp_UNKNOWN"), siwc.ErrUnknownAccount)
	assert.Equal(t, siwctest.IssuedClient, e.Reg(t).Active, "失敗したら選択は変わらない")
}

func TestSelectAccountRejectsASignedOutAccount(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	_, err := e.M.SignOut(t.Context(), siwctest.IssuedClient)
	require.NoError(t, err)
	assert.ErrorIs(t, e.M.SelectAccount(t.Context(), siwctest.IssuedClient), siwc.ErrNotSignedIn)
	assert.Equal(t, secondClient, e.Reg(t).Active)
}

// 片方をサインアウトしても、もう片方のトークンと登録は残る。失効するのも選んだアカウントのトークンだけ。
func TestSignOutAffectsOnlyTheSelectedAccount(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	require.NoError(t, e.M.SelectAccount(t.Context(), siwctest.IssuedClient))

	revoked, err := e.M.SignOut(t.Context(), secondClient) // 使用中でない方をログアウト
	require.NoError(t, err)
	assert.True(t, revoked)
	forms := e.F.RevokeFormsCopy()
	require.Len(t, forms, 1)
	assert.Equal(t, secondClient, forms[0].Get("client_id"))
	assert.Equal(t, siwctest.RefreshTokenFor(secondClient), forms[0].Get("token"), "選んだアカウントのリフレッシュトークンだけを失効させる")

	for _, k := range []string{"access", "refresh", "id"} {
		_, err := e.Sec.Get(secondTokenKey(k))
		assert.ErrorIs(t, err, secrets.ErrNotFound, "ログアウトしたアカウントの %s は消える", k)
	}
	got, err := e.Sec.Get(siwctest.TokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretRefresh, got, "もう片方のトークンは残る")
	_, err = e.Sec.Get(siwctest.TokenKey("id"))
	require.NoError(t, err)

	reg := e.Reg(t)
	require.Len(t, reg.Accounts, 2, "どちらの登録も残る")
	assert.Equal(t, siwctest.IssuedClient, reg.Active, "使用中のアカウントは変わらない")
	tok, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess, tok)
	st, err := e.M.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []siwc.Account{
		{ID: siwctest.IssuedClient, Email: "me@chatgpt.example", SignedIn: true, Active: true, PlanNoticePending: true},
		{ID: secondClient, Email: "me@chatgpt.example", PlanNoticePending: true},
	}, st.Accounts)
}

func TestSigningOutTheActiveAccountLeavesNoneActiveAndKeepsTheOther(t *testing.T) {
	e := twoAccounts(t, nil, nil) // 使用中は secondClient
	_, err := e.M.SignOut(t.Context(), secondClient)
	require.NoError(t, err)

	st, err := e.M.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, st.SignedIn, "使用中のアカウントがログアウトしたら、未ログイン（ほかのアカウントへ黙って切り替えない）")
	_, err = e.M.AccessToken(t.Context())
	assert.ErrorIs(t, err, siwc.ErrNotSignedIn)

	require.NoError(t, e.M.SelectAccount(t.Context(), siwctest.IssuedClient), "残したアカウントは選び直せる")
	tok, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess, tok)
}

func TestSignOutOfAnUnknownAccountChangesNothing(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	_, err := e.M.SignOut(t.Context(), "oaiapp_UNKNOWN")
	assert.ErrorIs(t, err, siwc.ErrUnknownAccount)
	_, err = e.M.SignOut(t.Context(), "")
	assert.ErrorIs(t, err, siwc.ErrUnknownAccount, "アカウントの指定は必須（使用中を暗黙に消さない）")
	assert.Empty(t, e.F.RevokeFormsCopy())
	st, _ := e.M.Status(t.Context())
	assert.True(t, st.Accounts[0].SignedIn)
	assert.True(t, st.Accounts[1].SignedIn)
}

// 更新は使用中のアカウントの client_id・リフレッシュトークンで行い、失敗で消えるのもそのアカウントのトークンだけ。
func TestRefreshUsesAndAffectsOnlyTheActiveAccount(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	e.Clk.Advance(2 * time.Hour)

	_, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	forms := e.F.TokenFormsCopy()
	last := forms[len(forms)-1]
	assert.Equal(t, "refresh_token", last.Get("grant_type"))
	assert.Equal(t, secondClient, last.Get("client_id"))
	assert.Equal(t, siwctest.RefreshTokenFor(secondClient), last.Get("refresh_token"))
	got, err := e.Sec.Get(siwctest.TokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretRefresh, got, "使用中でないアカウントのトークンは更新で変わらない")

	e.F.Mu.Lock()
	e.F.RefreshStatus, e.F.RefreshBody = http.StatusBadRequest, `{"error":"invalid_grant"}`
	e.F.Mu.Unlock()
	e.Clk.Advance(2 * time.Hour)
	_, err = e.M.AccessToken(t.Context())
	require.ErrorIs(t, err, siwc.ErrReauthRequired)
	_, err = e.Sec.Get(secondTokenKey("refresh"))
	assert.ErrorIs(t, err, secrets.ErrNotFound, "失敗したアカウントのトークンは消す")
	got, err = e.Sec.Get(siwctest.TokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretRefresh, got, "もう片方のトークンは残る")
	assert.Len(t, e.Reg(t).Accounts, 2)

	// 選び直せば、もう片方で更新できる（更新は選んだアカウントの client_id）。
	e.F.Mu.Lock()
	e.F.RefreshStatus = 0
	e.F.Mu.Unlock()
	require.NoError(t, e.M.SelectAccount(t.Context(), siwctest.IssuedClient))
	_, err = e.M.AccessToken(t.Context())
	require.NoError(t, err)
	forms = e.F.TokenFormsCopy()
	assert.Equal(t, siwctest.IssuedClient, forms[len(forms)-1].Get("client_id"))
}

// 再ログインは選んだ登録の client_id・ID トークン・メールだけを使い、ほかのアカウントのものを送らない。
func TestReloginUsesOnlyTheSelectedAccountsHints(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	idA, err := e.Sec.Get(siwctest.TokenKey("id"))
	require.NoError(t, err)
	idB, err := e.Sec.Get(secondTokenKey("id"))
	require.NoError(t, err)
	require.NotEqual(t, idA, idB)

	e.F.Mu.Lock()
	e.F.Subject = siwctest.SubjectA
	e.F.Mu.Unlock()
	st, err := e.M.SignIn(t.Context(), siwc.SignInOptions{AccountID: siwctest.IssuedClient})
	require.NoError(t, err)
	q := e.F.LastAuth()
	assert.Equal(t, siwctest.IssuedClient, q.Get("client_id"))
	assert.False(t, q.Has("agent_name_hint"))
	assert.NotEmpty(t, q.Get("id_token_hint"))
	assert.NotEqual(t, idB, q.Get("id_token_hint"), "ほかのアカウントの ID トークンは送らない")
	assert.Equal(t, "me@chatgpt.example", q.Get("login_hint"))
	assert.Equal(t, siwctest.IssuedClient, e.Reg(t).Active, "ログインを終えたアカウントが使用中になる")
	assert.True(t, st.SignedIn)

	// 別のアカウントの登録が選ばれたまま、メールだけが同じ別人でログインされても、sub が違えば保存しない。
	e.F.Mu.Lock()
	e.F.Subject = "sub-someone-else"
	e.F.Mu.Unlock()
	_, err = e.M.SignIn(t.Context(), siwc.SignInOptions{AccountID: secondClient})
	assert.ErrorIs(t, err, siwc.ErrAccountMismatch)
	assert.Equal(t, "sub-account-b", e.Reg(t).Accounts[1].Subject)

	_, err = e.M.SignIn(t.Context(), siwc.SignInOptions{AccountID: "oaiapp_UNKNOWN"})
	assert.ErrorIs(t, err, siwc.ErrUnknownAccount)
}

// 別のアカウントの追加で ChatGPT プランが許可されなかったとき、使用中のアカウントとそのトークンは変えない。
func TestPlanDeclinedWhileAddingAnAccountKeepsTheActiveSession(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)
	e.F.Mu.Lock()
	e.F.Scope = "email openid profile offline_access"
	e.F.Subject = "sub-account-b"
	e.F.IssueQueue = []string{secondClient}
	e.F.Mu.Unlock()

	_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrPlanNotGranted)

	tok, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess, tok)
	reg := e.Reg(t)
	assert.Equal(t, siwctest.IssuedClient, reg.Active)
	require.Len(t, reg.Accounts, 2)
	assert.True(t, reg.Accounts[1].ConsentNeeded, "追加しようとした登録は、次のログインで同意を求め直すために残す")
	assert.False(t, reg.Accounts[0].ConsentNeeded)
	_, err = e.Sec.Get(secondTokenKey("refresh"))
	assert.ErrorIs(t, err, secrets.ErrNotFound, "使わないトークンは保存しない")
	st, _ := e.M.Status(t.Context())
	assert.False(t, st.Accounts[1].SignedIn)
}
