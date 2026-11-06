package siwctests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/siwc/siwctest"
)

func pendingNotices(t *testing.T, e *siwctest.Env) map[string]bool {
	t.Helper()
	st, err := e.M.Status(t.Context())
	require.NoError(t, err)
	out := map[string]bool{}
	for _, a := range st.Accounts {
		out[a.ID] = a.PlanNoticePending
	}
	return out
}

// 初めてログインして ChatGPT プランの利用が有効になったときだけ確認を出す。再ログインでは出ない（アカウントごとに永続）。
func TestPlanNoticeIsShownOnlyOnTheFirstSignInOfAnAccount(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	st := e.SignIn(t)
	assert.True(t, st.Accounts[0].PlanNoticePending, "初めてのログイン")
	assert.Equal(t, "pending", e.Reg(t).First().PlanNotice)

	// 確認していない間は、再ログインしても立ったまま（出し直し）で、2 つ目の別の印は付かない。
	e.Relogin(t)
	assert.Equal(t, map[string]bool{siwctest.IssuedClient: true}, pendingNotices(t, e))

	require.NoError(t, e.M.AckPlanNotice(t.Context(), siwctest.IssuedClient))
	assert.Equal(t, map[string]bool{siwctest.IssuedClient: false}, pendingNotices(t, e))
	require.NoError(t, e.M.AckPlanNotice(t.Context(), siwctest.IssuedClient), "何度でも呼べる")

	// アプリの再起動（新しい Manager・同じ保存先）でも、ログアウトと再ログインでも、出ない。
	m2 := siwc.New(siwc.Options{OpenBrowser: e.F.Browser(t), Secrets: e.Sec, Settings: e.Settings, Clock: e.Clk, Issuer: e.F.Srv.URL, HTTPClient: e.F.Srv.Client()})
	st2, err := m2.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, st2.Accounts[0].PlanNoticePending)
	_, err = e.M.SignOut(t.Context(), siwctest.IssuedClient)
	require.NoError(t, err)
	assert.False(t, e.Relogin(t).Accounts[0].PlanNoticePending, "再ログインでは出さない")

	// 2 つ目のアカウントは初めてなので 1 度だけ出る。1 つ目は変わらない。
	e.F.Mu.Lock()
	e.F.IssueQueue = []string{secondClient}
	e.F.Subject = "sub-account-b"
	e.F.Mu.Unlock()
	st, err = e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{siwctest.IssuedClient: false, secondClient: true}, pendingNotices(t, e))
	require.NoError(t, e.M.AckPlanNotice(t.Context(), secondClient))
	assert.Equal(t, map[string]bool{siwctest.IssuedClient: false, secondClient: false}, pendingNotices(t, e))
	assert.Len(t, st.Accounts, 2)

	assert.ErrorIs(t, e.M.AckPlanNotice(t.Context(), "oaiapp_UNKNOWN"), siwc.ErrUnknownAccount)
}

// ChatGPT プランの利用が許可されなかったログインでは出さない。あとで許可されて初めて有効になったときに出す。
func TestPlanNoticeWaitsUntilThePlanIsActuallyEnabled(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.F.Scope = "email openid profile offline_access"
	_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrPlanNotGranted)
	assert.Equal(t, map[string]bool{siwctest.IssuedClient: false}, pendingNotices(t, e), "許可されなかったときは出さない")
	assert.Empty(t, e.Reg(t).First().PlanNotice)

	e.F.Mu.Lock()
	e.F.Scope = siwctest.ScopeAll
	e.F.Mu.Unlock()
	st := e.Relogin(t)
	assert.True(t, st.Accounts[0].PlanNoticePending, "ChatGPT プランの利用が初めて有効になったので出す")
}

// 更新（リフレッシュ）では、確認の状態は変わらない。
func TestPlanNoticeSurvivesTokenRefresh(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)
	require.NoError(t, e.M.AckPlanNotice(t.Context(), siwctest.IssuedClient))
	_, err := e.M.ForceRefresh(t.Context())
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{siwctest.IssuedClient: false}, pendingNotices(t, e))
}
