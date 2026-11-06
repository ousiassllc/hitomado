package accounttests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/store"
)

// 認可コード交換で Client ID／Secret が拒否されたら、INTERNAL ではなく OAUTH_CLIENT_REJECTED を返す。
func TestStartAddAccountClientRejected(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "x@a.example")
	e.Flow.Err = fmt.Errorf("oauth: トークン交換に失敗しました: %w", oauth.ErrClientRejected)

	_, err := svc.StartAddAccount(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.OAuthClientRejected, err)
}

func TestReconnectClientRejected(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	require.NoError(t, e.Store.SetAccountStatus(t.Context(), a.ID, store.StatusNeedsReauth, "expired"))
	e.Flow.Err = fmt.Errorf("oauth: トークン交換に失敗しました: %w", oauth.ErrClientRejected)

	_, err := svc.Reconnect(t.Context(), a.ID)
	_ = requireCode(t, apperr.OAuthClientRejected, err)
	got, _ := e.Store.GetAccount(t.Context(), a.ID)
	assert.Equal(t, store.StatusNeedsReauth, got.Status)
}
