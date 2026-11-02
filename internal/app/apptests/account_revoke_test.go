package apptests

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

// failingSet は refresh token の保存だけ失敗させる secrets.Store。
type failingSet struct{ secrets.Store }

func (f failingSet) Set(key, value string) error {
	if strings.Contains(key, "refresh_token") {
		return errors.New("keyring unavailable")
	}
	return f.Store.Set(key, value)
}

func TestStartAddAccountDuplicateRevokesOnlyWhenGrantNotShared(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "dup@a.example")
	other := e.AddOAuthClient(t, "client-2")

	// 同じクライアントの重複: 既存アカウントの認可と共有されるので失効させない。
	e.Flow.Result = oauth.Result{Email: "dup@a.example", RefreshToken: "rt-same-client"}
	_, err := svc.StartAddAccount(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.AccountDuplicate, err)
	assert.Empty(t, e.Flow.Revoked)

	// 別クライアントの重複: 保存しないトークンを失効させる。
	e.Flow.Result = oauth.Result{Email: "dup@a.example", RefreshToken: "rt-other-client"}
	e.Flow.RevokeErr = assert.AnError // 失敗は無視
	_, err = svc.StartAddAccount(t.Context(), other.ID)
	_ = requireCode(t, apperr.AccountDuplicate, err)
	assert.Equal(t, []string{"rt-other-client"}, e.Flow.Revoked)
	tok, _ := e.Secrets.Get(secrets.RefreshTokenKey(a.ID))
	assert.Equal(t, "refresh-dup@a.example", tok)
}

func TestStartAddAccountSecretsFailureRollsBackAndRevokes(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "x@a.example")
	e.Deps.Secrets = failingSet{e.Secrets}
	e.Flow.Result = oauth.Result{Email: "new@b.example", RefreshToken: "rt-new"}

	_, err := svc.StartAddAccount(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.Internal, err)
	assert.Equal(t, []string{"rt-new"}, e.Flow.Revoked)
	list, _ := svc.ListAccounts(t.Context())
	assert.Len(t, list, 1, "作りかけのアカウントを残さない")
	assert.Empty(t, e.Sync.StartedAccounts())
}

func TestStartAddAccountIncompleteResultRevokes(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "x@a.example")
	e.Flow.Result = oauth.Result{RefreshToken: "rt-no-email"} //nolint:gosec // テスト用のダミー値
	_, err := svc.StartAddAccount(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.GmailError, err)
	assert.Equal(t, []string{"rt-no-email"}, e.Flow.Revoked)
}

func TestReconnectRevokesTokenItCannotKeep(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	require.NoError(t, e.Store.SetAccountStatus(t.Context(), a.ID, store.StatusNeedsReauth, "expired"))

	e.Flow.Result = oauth.Result{Email: "other@a.example", RefreshToken: "rt-other"}
	_, err := svc.Reconnect(t.Context(), a.ID)
	_ = requireCode(t, apperr.Validation, err)
	assert.Equal(t, []string{"rt-other"}, e.Flow.Revoked, "別アドレスのトークンは失効させる")

	e.Deps.Secrets = failingSet{e.Secrets}
	e.Flow.Result = oauth.Result{Email: "me@a.example", RefreshToken: "rt-unsaved"}
	_, err = svc.Reconnect(t.Context(), a.ID)
	_ = requireCode(t, apperr.Internal, err)
	assert.Equal(t, []string{"rt-other", "rt-unsaved"}, e.Flow.Revoked)
	got, _ := e.Store.GetAccount(t.Context(), a.ID)
	assert.Equal(t, store.StatusNeedsReauth, got.Status)
}

func TestReconnectMismatchKeepsGrantSharedWithOtherAccount(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	e.AddAccount(t, "other@a.example") // 同じクライアントで別アドレスのアカウントが接続済み
	e.Flow.Result = oauth.Result{Email: "other@a.example", RefreshToken: "rt-shared"}
	_, err := svc.Reconnect(t.Context(), a.ID)
	_ = requireCode(t, apperr.Validation, err)
	assert.Empty(t, e.Flow.Revoked, "接続済みの other@ の認可を巻き込まない")
}

func TestForgetOnRemoveAndOAuthClientUpdate(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	other := e.AddOAuthClient(t, "client-2")
	require.NoError(t, e.Store.SetAccountStatus(t.Context(), b.ID, store.StatusActive, ""))

	_, err := svc.UpdateOAuthClient(t.Context(), a.OAuthClientID, "n", "default-client", "rotated")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{a.ID, b.ID}, e.Gmail.Forgotten, "そのクライアントを使う全アカウント")

	e.Gmail.Forgotten = nil
	_, err = svc.UpdateOAuthClient(t.Context(), other.ID, "n", "client-2", "")
	require.NoError(t, err)
	assert.Empty(t, e.Gmail.Forgotten, "使うアカウントが無ければ何もしない")

	require.NoError(t, svc.RemoveAccount(t.Context(), a.ID))
	assert.Equal(t, []string{a.ID}, e.Gmail.Forgotten)
	assert.Equal(t, []string{a.ID}, e.Sync.StoppedAccounts())
}
