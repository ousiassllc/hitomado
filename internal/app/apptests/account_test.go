package apptests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

func TestStartAddAccount(t *testing.T) {
	svc, e := newAccountSvc(t)
	first := e.AddAccount(t, "first@a.example")
	cid := first.OAuthClientID
	e.Flow.Result = oauth.Result{Email: "New@B.example", Name: "New Person", RefreshToken: "rt-new"}

	acc, err := svc.StartAddAccount(t.Context(), cid)
	require.NoError(t, err)
	assert.Equal(t, "new@b.example", acc.Email)
	assert.Equal(t, "New Person", acc.DisplayName)
	assert.NotEqual(t, first.Color, acc.Color, "未使用の色を割り当てる")
	assert.Equal(t, "active", acc.Status)
	assert.Equal(t, []string{acc.ID}, e.Sync.StartedAccounts())
	tok, err := e.Secrets.Get(secrets.RefreshTokenKey(acc.ID))
	require.NoError(t, err)
	assert.Equal(t, "rt-new", tok)
	require.Len(t, e.Flow.Configs, 1)
	assert.Equal(t, "default-client", e.Flow.Configs[0].ClientID)
	assert.Equal(t, "secret-default-client", e.Flow.Configs[0].ClientSecret)

	list, err := svc.ListAccounts(t.Context())
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestStartAddAccountDuplicateKeepsNoToken(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "dup@a.example")
	e.Flow.Result = oauth.Result{Email: "DUP@a.example", Name: "x", RefreshToken: "rt-dup"}

	_, err := svc.StartAddAccount(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.AccountDuplicate, err)

	tok, _ := e.Secrets.Get(secrets.RefreshTokenKey(a.ID))
	assert.Equal(t, "refresh-dup@a.example", tok, "既存トークンを上書きしない")
	assert.Empty(t, e.Sync.StartedAccounts())
	list, _ := svc.ListAccounts(t.Context())
	assert.Len(t, list, 1)
}

func TestStartAddAccountNameFallsBackToLocalPart(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "x@a.example")
	e.Flow.Result = oauth.Result{Email: "taro@b.example", RefreshToken: "rt"}
	acc, err := svc.StartAddAccount(t.Context(), a.OAuthClientID)
	require.NoError(t, err)
	assert.Equal(t, "taro", acc.DisplayName)
}

func TestStartAddAccountErrors(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "x@a.example")

	e.Flow.Err = oauth.ErrDenied
	_, err := svc.StartAddAccount(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.OAuthDenied, err)
	e.Flow.Err = oauth.ErrTimeout
	_, err = svc.StartAddAccount(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.OAuthTimeout, err)

	_, err = svc.StartAddAccount(t.Context(), "missing")
	_ = requireCode(t, apperr.NotFound, err)

	require.NoError(t, e.Secrets.Delete(secrets.OAuthClientSecretKey(a.OAuthClientID)))
	_, err = svc.StartAddAccount(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.Validation, err)
}

func waitStarted(t *testing.T, e *apptest.Env) {
	t.Helper()
	select {
	case <-e.Flow.Started:
	case <-time.After(5 * time.Second):
		t.Fatal("認可フローが開始されませんでした")
	}
}

func TestCancelAddAccount(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "x@a.example")
	e.Flow.Block = true

	done := make(chan error, 1)
	go func() {
		_, err := svc.StartAddAccount(context.Background(), a.OAuthClientID)
		done <- err
	}()
	waitStarted(t, e)

	// 同時に 2 つ開始しない
	_, err := svc.StartAddAccount(t.Context(), a.OAuthClientID)
	_ = requireCode(t, apperr.Validation, err)

	require.NoError(t, svc.CancelAddAccount())
	_ = requireCode(t, apperr.OAuthCancelled, <-done)

	// 完了後は再び開始できる
	e.Flow.Block = false
	e.Flow.Result = oauth.Result{Email: "n@b.example", RefreshToken: "rt"}
	_, err = svc.StartAddAccount(t.Context(), a.OAuthClientID)
	require.NoError(t, err)
	require.NoError(t, svc.CancelAddAccount(), "進行中でなくても無害")
}

func TestStartAddAccountContextCancel(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "x@a.example")
	e.Flow.Block = true
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, err := svc.StartAddAccount(ctx, a.OAuthClientID)
		done <- err
	}()
	waitStarted(t, e)
	cancel()
	_ = requireCode(t, apperr.OAuthCancelled, <-done)
	assert.Empty(t, e.Sync.StartedAccounts())
}

func TestReconnect(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	require.NoError(t, e.Store.SetAccountStatus(t.Context(), a.ID, store.StatusNeedsReauth, "expired"))
	e.Flow.Result = oauth.Result{Email: "ME@a.example", RefreshToken: "rt-fresh"}

	acc, err := svc.Reconnect(t.Context(), a.ID)
	require.NoError(t, err)
	assert.Equal(t, "active", acc.Status)
	assert.Empty(t, acc.StatusMessage)
	tok, _ := e.Secrets.Get(secrets.RefreshTokenKey(a.ID))
	assert.Equal(t, "rt-fresh", tok)
	assert.Equal(t, []string{a.ID}, e.Sync.StartedAccounts())
	assert.Equal(t, []string{a.ID}, e.Gmail.Forgotten)
	evs := e.Events.Named(events.AccountStatus)
	require.Len(t, evs, 1)
	assert.Equal(t, events.AccountStatusPayload{AccountID: a.ID, Status: "active"}, evs[0].Payload)
}

func TestReconnectRejectsDifferentAddress(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	require.NoError(t, e.Store.SetAccountStatus(t.Context(), a.ID, store.StatusNeedsReauth, "expired"))
	e.Flow.Result = oauth.Result{Email: "other@a.example", RefreshToken: "rt-other"}

	_, err := svc.Reconnect(t.Context(), a.ID)
	ae := requireCode(t, apperr.Validation, err)
	assert.Equal(t, a.ID, ae.AccountID)

	tok, _ := e.Secrets.Get(secrets.RefreshTokenKey(a.ID))
	assert.Equal(t, "refresh-me@a.example", tok, "トークンは保存しない")
	got, _ := e.Store.GetAccount(t.Context(), a.ID)
	assert.Equal(t, store.StatusNeedsReauth, got.Status)
	assert.Empty(t, e.Sync.StartedAccounts())
	assert.Empty(t, e.Events.Named(events.AccountStatus))

	_, err = svc.Reconnect(t.Context(), "missing")
	_ = requireCode(t, apperr.NotFound, err)
}

func TestRemoveAccountCascadesAndCleansUp(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	b := e.AddAccount(t, "me@b.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", apptest.Epoch, "INBOX"))
	e.PutThread(t, b.ID, "t2", apptest.Msg("m2", "t2", apptest.Epoch, "INBOX"))
	_, err := e.Deps.Threads.Get(t.Context(), a.ID, "t1")
	require.NoError(t, err)
	callsBefore := e.Gmail.CallCount("GetThreadFull")
	e.Flow.RevokeErr = assert.AnError // revoke の失敗は無視

	require.NoError(t, svc.RemoveAccount(t.Context(), a.ID))

	assert.Equal(t, []string{a.ID}, e.Sync.StoppedAccounts())
	_, err = e.Store.GetAccount(t.Context(), a.ID)
	assert.ErrorIs(t, err, store.ErrNotFound)
	_, err = e.Store.GetThread(t.Context(), a.ID, "t1")
	assert.ErrorIs(t, err, store.ErrNotFound, "スレッドもカスケード削除")
	_, err = e.Store.GetThread(t.Context(), b.ID, "t2")
	assert.NoError(t, err, "他アカウントは残る")
	_, err = e.Secrets.Get(secrets.RefreshTokenKey(a.ID))
	assert.ErrorIs(t, err, secrets.ErrNotFound)
	_, err = e.Secrets.Get(secrets.RefreshTokenKey(b.ID))
	assert.NoError(t, err)
	assert.Equal(t, []string{"refresh-me@a.example"}, e.Flow.Revoked)

	_, err = e.Deps.Threads.Get(t.Context(), a.ID, "t1")
	require.NoError(t, err)
	assert.Equal(t, callsBefore+1, e.Gmail.CallCount("GetThreadFull"), "キャッシュは破棄される")

	_ = requireCode(t, apperr.NotFound, svc.RemoveAccount(t.Context(), a.ID))
}

func TestUpdateAccount(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	ctx := t.Context()

	name, color, off := " 新しい名前 ", "teal", false
	acc, err := svc.UpdateAccount(ctx, a.ID, dto.AccountPatch{DisplayName: &name, Color: &color, NotifyEnabled: &off})
	require.NoError(t, err)
	assert.Equal(t, "新しい名前", acc.DisplayName)
	assert.Equal(t, "teal", acc.Color)
	assert.False(t, acc.NotifyEnabled)

	empty, bad := "  ", "magenta"
	_, err = svc.UpdateAccount(ctx, a.ID, dto.AccountPatch{DisplayName: &empty})
	_ = requireCode(t, apperr.Validation, err)
	_, err = svc.UpdateAccount(ctx, a.ID, dto.AccountPatch{Color: &bad})
	_ = requireCode(t, apperr.Validation, err)
	_, err = svc.UpdateAccount(ctx, "missing", dto.AccountPatch{Color: &color})
	_ = requireCode(t, apperr.NotFound, err)

	got, _ := e.Store.GetAccount(ctx, a.ID)
	assert.Equal(t, "新しい名前", got.DisplayName, "検証エラーでは変更されない")
}

func TestSetSignatureSanitizes(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "me@a.example")
	acc, err := svc.SetSignature(t.Context(), a.ID, `<p>Taro <strong>Y</strong></p><script>alert(1)</script><a href="javascript:x()">l</a>`)
	require.NoError(t, err)
	assert.Contains(t, acc.SignatureHTML, "<strong>Y</strong>")
	assert.NotContains(t, acc.SignatureHTML, "script")
	assert.NotContains(t, acc.SignatureHTML, "javascript")
	got, _ := e.Store.GetAccount(t.Context(), a.ID)
	assert.Equal(t, acc.SignatureHTML, got.SignatureHTML)
}

func TestReorderAccounts(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	c := e.AddAccount(t, "c@x.example")
	ctx := t.Context()

	out, err := svc.ReorderAccounts(ctx, []string{c.ID, a.ID, b.ID})
	require.NoError(t, err)
	require.Len(t, out, 3)
	assert.Equal(t, []string{c.ID, a.ID, b.ID}, []string{out[0].ID, out[1].ID, out[2].ID})
	assert.Equal(t, []int{0, 1, 2}, []int{out[0].SortOrder, out[1].SortOrder, out[2].SortOrder})

	for _, bad := range [][]string{{a.ID, b.ID}, {a.ID, b.ID, c.ID, "x"}, {a.ID, a.ID, b.ID}, {a.ID, b.ID, "zzz"}, nil} {
		_, err = svc.ReorderAccounts(ctx, bad)
		_ = requireCode(t, apperr.Validation, err)
	}
	list, _ := svc.ListAccounts(ctx)
	assert.Equal(t, c.ID, list[0].ID, "失敗した並べ替えは反映しない")
}

func TestListAccountsEmptyIsNotNil(t *testing.T) {
	svc, _ := newAccountSvc(t)
	list, err := svc.ListAccounts(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, list)
	assert.Empty(t, list)
}
