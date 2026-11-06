// Package accounttests は AccountService（アカウントの追加・再接続・削除・ログインの中断・クライアント拒否）を、
// 実 SQLite とフェイク（apptest）で検証する外部テスト。
package accounttests

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
)

// requireCode は err が want コードの apperr.Error であることを検証し、その *apperr.Error を返す。
func requireCode(t *testing.T, want string, err error) *apperr.Error {
	t.Helper()
	require.Error(t, err)
	var ae *apperr.Error
	require.True(t, errors.As(err, &ae), "apperr.Error ではない: %v", err)
	require.Equal(t, want, ae.Code)
	return ae
}

func newAccountSvc(t *testing.T) (*app.AccountService, *apptest.Env) {
	t.Helper()
	e := apptest.NewEnv(t)
	return app.NewAccountService(e.Deps), e
}
