package siwctests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/siwc"
)

func pendingNotices(t *testing.T, e *env) map[string]bool {
	t.Helper()
	st, err := e.m.Status(t.Context())
	require.NoError(t, err)
	out := map[string]bool{}
	for _, a := range st.Accounts {
		out[a.ID] = a.PlanNoticePending
	}
	return out
}

// 初めてログインして ChatGPT プランの利用が有効になったときだけ確認を出す。再ログインでは出ない（アカウントごとに永続）。
func TestPlanNoticeIsShownOnlyOnTheFirstSignInOfAnAccount(t *testing.T) {
	e := newEnv(t, nil, nil)
	st := e.signIn(t)
	assert.True(t, st.Accounts[0].PlanNoticePending, "初めてのログイン")
	assert.Equal(t, "pending", e.reg(t).first().PlanNotice)

	// 確認していない間は、再ログインしても立ったまま（出し直し）で、2 つ目の別の印は付かない。
	e.relogin(t)
	assert.Equal(t, map[string]bool{issuedClient: true}, pendingNotices(t, e))

	require.NoError(t, e.m.AckPlanNotice(t.Context(), issuedClient))
	assert.Equal(t, map[string]bool{issuedClient: false}, pendingNotices(t, e))
	require.NoError(t, e.m.AckPlanNotice(t.Context(), issuedClient), "何度でも呼べる")

	// アプリの再起動（新しい Manager・同じ保存先）でも、ログアウトと再ログインでも、出ない。
	m2 := siwc.New(siwc.Options{OpenBrowser: e.f.browser(t), Secrets: e.sec, Settings: e.settings, Clock: e.clk, Issuer: e.f.srv.URL, HTTPClient: e.f.srv.Client()})
	st2, err := m2.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, st2.Accounts[0].PlanNoticePending)
	_, err = e.m.SignOut(t.Context(), issuedClient)
	require.NoError(t, err)
	assert.False(t, e.relogin(t).Accounts[0].PlanNoticePending, "再ログインでは出さない")

	// 2 つ目のアカウントは初めてなので 1 度だけ出る。1 つ目は変わらない。
	e.f.mu.Lock()
	e.f.issueQueue = []string{secondClient}
	e.f.subject = "sub-account-b"
	e.f.mu.Unlock()
	st, err = e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{issuedClient: false, secondClient: true}, pendingNotices(t, e))
	require.NoError(t, e.m.AckPlanNotice(t.Context(), secondClient))
	assert.Equal(t, map[string]bool{issuedClient: false, secondClient: false}, pendingNotices(t, e))
	assert.Len(t, st.Accounts, 2)

	assert.ErrorIs(t, e.m.AckPlanNotice(t.Context(), "oaiapp_UNKNOWN"), siwc.ErrUnknownAccount)
}

// ChatGPT プランの利用が許可されなかったログインでは出さない。あとで許可されて初めて有効になったときに出す。
func TestPlanNoticeWaitsUntilThePlanIsActuallyEnabled(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.scope = "email openid profile offline_access"
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrPlanNotGranted)
	assert.Equal(t, map[string]bool{issuedClient: false}, pendingNotices(t, e), "許可されなかったときは出さない")
	assert.Empty(t, e.reg(t).first().PlanNotice)

	e.f.mu.Lock()
	e.f.scope = scopeAll
	e.f.mu.Unlock()
	st := e.relogin(t)
	assert.True(t, st.Accounts[0].PlanNoticePending, "ChatGPT プランの利用が初めて有効になったので出す")
}

// 更新（リフレッシュ）では、確認の状態は変わらない。
func TestPlanNoticeSurvivesTokenRefresh(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	require.NoError(t, e.m.AckPlanNotice(t.Context(), issuedClient))
	_, err := e.m.ForceRefresh(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{issuedClient: false}, pendingNotices(t, e))
}
