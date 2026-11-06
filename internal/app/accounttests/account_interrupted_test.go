package accounttests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/oauth"
)

// 検出しない構成（Windows。OAuthTracker が nil）は常に false。
func TestTakeInterruptedAuthWithoutTracker(t *testing.T) {
	svc, e := newAccountSvc(t)
	require.Nil(t, e.Deps.OAuthTracker)
	assert.False(t, svc.TakeInterruptedAuth())
}

// 前回の起動が認可の途中で止められていたら 1 回だけ true。そのあとの認可は普通に完了する。
func TestTakeInterruptedAuthReportsOnce(t *testing.T) {
	svc, e := newAccountSvc(t)
	// 前回の起動: 認可を始めて（印を残して）、完了前にプロセスが止められた。
	oauth.NewTracker(e.Secrets).Begin()

	e.Deps.OAuthTracker = oauth.NewTracker(e.Secrets) // 次の起動
	assert.True(t, svc.TakeInterruptedAuth())
	assert.False(t, svc.TakeInterruptedAuth())

	c := e.AddOAuthClient(t, "c1")
	e.Flow.Result = oauth.Result{Email: "again@a.example", Name: "Again", RefreshToken: "rt-again"}
	acc, err := svc.StartAddAccount(t.Context(), c.ID)
	require.NoError(t, err, "やり直したログインは完了できる")
	assert.Equal(t, "again@a.example", acc.Email)
}
