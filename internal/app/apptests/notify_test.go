package apptests

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/notify"
	"github.com/ousiassllc/hitomado/internal/oauth"
)

func TestFirstAccountAddSchedulesSyncAndRequestsPermission(t *testing.T) {
	svc, e := newAccountSvc(t)
	cid := e.AddOAuthClient(t, "c").ID
	e.Flow.Result = oauth.Result{Email: "one@a.example", RefreshToken: "rt1"}
	_, err := svc.StartAddAccount(t.Context(), cid)
	require.NoError(t, err)

	schedules, _, requests := e.Background.Snapshot()
	assert.Equal(t, []bool{true}, schedules, "追加後にバックグラウンド同期を登録する")
	assert.Equal(t, 1, requests, "最初のアカウントの追加が済んだところで通知の許可を求める")

	e.Flow.Result = oauth.Result{Email: "two@a.example", RefreshToken: "rt2"}
	_, err = svc.StartAddAccount(t.Context(), cid)
	require.NoError(t, err)
	schedules, _, requests = e.Background.Snapshot()
	assert.Equal(t, []bool{true, true}, schedules, "登録は冪等（WorkManager 側が KEEP）")
	assert.Equal(t, 1, requests, "2 件目以降では許可を求め直さない")
}

func TestFailedAccountAddDoesNotTouchBackground(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "dup@a.example")
	e.Flow.Result = oauth.Result{Email: "dup@a.example", RefreshToken: "rt"}
	_, err := svc.StartAddAccount(t.Context(), a.OAuthClientID)
	require.Error(t, err)
	schedules, _, requests := e.Background.Snapshot()
	assert.Empty(t, schedules)
	assert.Zero(t, requests)
}

func TestRemoveAccountForgetsChannelAndUnschedulesWhenLast(t *testing.T) {
	svc, e := newAccountSvc(t)
	a := e.AddAccount(t, "a@a.example")
	b := e.AddAccount(t, "b@a.example")

	require.NoError(t, svc.RemoveAccount(t.Context(), a.ID))
	schedules, forgotten, _ := e.Background.Snapshot()
	assert.Equal(t, []string{a.ID}, forgotten, "削除したアカウントの通知チャネルを消す")
	assert.Empty(t, schedules, "ほかのアカウントが残るうちは同期を解除しない")

	require.NoError(t, svc.RemoveAccount(t.Context(), b.ID))
	schedules, forgotten, _ = e.Background.Snapshot()
	assert.Equal(t, []string{a.ID, b.ID}, forgotten)
	assert.Equal(t, []bool{false}, schedules, "0 件になったらバックグラウンド同期を解除する")
}

func TestNotifyServicePermission(t *testing.T) {
	e := apptest.NewEnv(t)
	svc := app.NewNotifyService(e.Deps)
	assert.Equal(t, "granted", svc.GetNotificationPermission())

	e.Background.State = notify.PermissionDenied
	assert.Equal(t, "denied", svc.GetNotificationPermission())

	svc.RequestNotificationPermission()
	_, _, requests := e.Background.Snapshot()
	assert.Equal(t, 1, requests)

	require.NoError(t, svc.OpenNotificationSettings())
	assert.True(t, e.Background.SettingsOK)

	e.Background.OpenErr = errors.New("no settings activity")
	_ = requireCode(t, apperr.Unsupported, svc.OpenNotificationSettings())
}

func TestNotifyServiceWithoutBackgroundIsUnsupported(t *testing.T) {
	e := apptest.NewEnv(t)
	e.Deps.Background = nil
	svc := app.NewNotifyService(e.Deps)
	assert.Equal(t, "unsupported", svc.GetNotificationPermission(), "Windows では許可の項目を出さない")
	svc.RequestNotificationPermission()
	assert.NoError(t, svc.OpenNotificationSettings())
	assert.Nil(t, svc.TakePendingOpenThread())
}

func TestTakePendingOpenThread(t *testing.T) {
	e := apptest.NewEnv(t)
	var delivered []notify.OpenRef
	e.Deps.OpenRequests = notify.NewOpenRequests(func(r notify.OpenRef) { delivered = append(delivered, r) })
	svc := app.NewNotifyService(e.Deps)

	e.Deps.OpenRequests.Offer(notify.OpenRef{AccountID: "a", ThreadID: "t1"}) // フロントの準備前（コールドスタートでのタップ）
	ref := svc.TakePendingOpenThread()
	require.NotNil(t, ref)
	assert.Equal(t, "a", ref.AccountID)
	assert.Equal(t, "t1", ref.ThreadID)
	assert.Nil(t, svc.TakePendingOpenThread())

	e.Deps.OpenRequests.Offer(notify.OpenRef{AccountID: "a", ThreadID: "t2"})
	assert.Equal(t, []notify.OpenRef{{AccountID: "a", ThreadID: "t2"}}, delivered, "準備後はそのまま発行する")
}
