package apptests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/update"
)

func newAppSvc(t *testing.T) (*app.AppService, *apptest.Env) {
	t.Helper()
	e := apptest.NewEnv(t)
	return app.NewAppService(e.Deps), e
}

func TestInboxFilterRoundTripDropsUnknownAccounts(t *testing.T) {
	svc, e := newAppSvc(t)
	ctx := t.Context()
	f, err := svc.GetInboxFilter(ctx)
	require.NoError(t, err)
	assert.Equal(t, dto.InboxFilter{AccountIDs: []string{}}, f, "未保存は全アカウント・未読のみなし")

	a := e.AddAccount(t, "a@x.example")
	b := e.AddAccount(t, "b@x.example")
	require.NoError(t, svc.SetInboxFilter(ctx, dto.InboxFilter{AccountIDs: []string{a.ID, "ghost", a.ID}, UnreadOnly: true}))
	f, err = svc.GetInboxFilter(ctx)
	require.NoError(t, err)
	assert.Equal(t, dto.InboxFilter{AccountIDs: []string{a.ID}, UnreadOnly: true}, f)

	require.NoError(t, svc.SetInboxFilter(ctx, dto.InboxFilter{AccountIDs: []string{a.ID, b.ID}}))
	require.NoError(t, app.NewAccountService(e.Deps).RemoveAccount(ctx, a.ID))
	f, err = svc.GetInboxFilter(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{b.ID}, f.AccountIDs, "削除済みアカウントは取得時にも除く")
}

func TestInboxFilterCorruptValueFallsBack(t *testing.T) {
	svc, e := newAppSvc(t)
	require.NoError(t, e.Store.SetSetting(t.Context(), "inbox.filter", `"not an object"`))
	f, err := svc.GetInboxFilter(t.Context())
	require.NoError(t, err)
	assert.Equal(t, dto.InboxFilter{AccountIDs: []string{}}, f)
}

func TestAppInfoAndUpdateToggle(t *testing.T) {
	svc, e := newAppSvc(t)
	ctx := t.Context()
	info, err := svc.GetAppInfo(ctx)
	require.NoError(t, err)
	assert.Equal(t, dto.AppInfo{Version: "0.1.0", UpdateCheckEnabled: true, DataDir: e.Deps.DataDir}, info)

	require.NoError(t, svc.SetUpdateCheckEnabled(ctx, false))
	info, _ = svc.GetAppInfo(ctx)
	assert.False(t, info.UpdateCheckEnabled)
	require.NoError(t, svc.SetUpdateCheckEnabled(ctx, true))
	info, _ = svc.GetAppInfo(ctx)
	assert.True(t, info.UpdateCheckEnabled)
}

func TestCheckForUpdate(t *testing.T) {
	svc, e := newAppSvc(t)
	ctx := t.Context()
	e.Update.Release = update.Release{Version: "0.2.0", URL: "https://example.com/r"}

	info, err := svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	assert.Equal(t, dto.UpdateInfo{Available: true, Version: "0.2.0", URL: "https://example.com/r"}, info)

	e.Update.Release = update.Release{Version: "0.1.0", URL: "u"}
	info, err = svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	assert.Equal(t, dto.UpdateInfo{}, info, "同じ版は新版ではない")

	e.Update.Release = update.Release{Version: "0.0.9"}
	info, _ = svc.CheckForUpdate(ctx)
	assert.False(t, info.Available, "古い版は新版ではない")

	calls := e.Update.CallCount()
	require.NoError(t, svc.SetUpdateCheckEnabled(ctx, false))
	e.Update.Release = update.Release{Version: "9.9.9"}
	info, err = svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	assert.Equal(t, dto.UpdateInfo{}, info)
	assert.Equal(t, calls, e.Update.CallCount(), "無効時は通信しない")

	require.NoError(t, svc.SetUpdateCheckEnabled(ctx, true))
	e.Update.Err = gmail.ErrOffline
	_, err = svc.CheckForUpdate(ctx)
	_ = requireCode(t, apperr.Offline, err)
}

// tickerFor は手動で刻める ticker を Deps に差し込む。
func tickerFor(e *apptest.Env) chan time.Time {
	ch := make(chan time.Time)
	e.Deps.UpdateTicker = func() (<-chan time.Time, func()) { return ch, func() {} }
	return ch
}

func runLoop(t *testing.T, svc *app.AppService) (cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	d := make(chan struct{})
	go func() { defer close(d); svc.RunUpdateLoop(ctx) }()
	t.Cleanup(func() { cancel(); <-d })
	return cancel, d
}

func waitEvents(t *testing.T, e *apptest.Env, n int) []apptest.Emitted {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	got := e.Events.Wait(ctx, events.UpdateAvailable, n)
	require.GreaterOrEqual(t, len(got), n, "update:available が %d 回発行されること", n)
	return got
}

func TestRunUpdateLoopEmitsAtStartAndOnTick(t *testing.T) {
	svc, e := newAppSvc(t)
	tick := tickerFor(e)
	e.Update.Release = update.Release{Version: "0.3.0", URL: "https://example.com/r"}
	cancel, done := runLoop(t, svc)

	got := waitEvents(t, e, 1)
	assert.Equal(t, events.UpdateAvailablePayload{Version: "0.3.0", URL: "https://example.com/r"}, got[0].Payload)

	tick <- time.Now() // ループが次の確認待ちに入っていれば受け取る
	waitEvents(t, e, 2)
	assert.Equal(t, 2, e.Update.CallCount())

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunUpdateLoop が ctx キャンセルで終了しない")
	}
}

func TestRunUpdateLoopSkipsWhenDisabled(t *testing.T) {
	svc, e := newAppSvc(t)
	tick := tickerFor(e)
	require.NoError(t, svc.SetUpdateCheckEnabled(t.Context(), false))
	e.Update.Release = update.Release{Version: "9.0.0"}
	runLoop(t, svc)

	tick <- time.Now() // 受け取られた時点で、起動時の確認は完了している
	tick <- time.Now() // 同様に 2 回目の確認が完了している
	assert.Zero(t, e.Update.CallCount(), "無効なら通信しない")
	assert.Empty(t, e.Events.Named(events.UpdateAvailable))
}

func TestRunUpdateLoopDoesNotEmitForSameVersion(t *testing.T) {
	svc, e := newAppSvc(t)
	tick := tickerFor(e)
	e.Update.Release = update.Release{Version: "0.1.0", URL: "u"}
	runLoop(t, svc)

	tick <- time.Now()
	tick <- time.Now()
	assert.GreaterOrEqual(t, e.Update.CallCount(), 2, "有効なら確認する")
	assert.Empty(t, e.Events.Named(events.UpdateAvailable), "同じ版では発行しない")
}

func TestRunUpdateLoopSurvivesCheckFailure(t *testing.T) {
	svc, e := newAppSvc(t)
	tick := tickerFor(e)
	e.Update.Err = gmail.ErrOffline
	runLoop(t, svc)
	tick <- time.Now() // 失敗した起動時確認の後でも次の周期を受け取る
	e.Update.Set(update.Release{Version: "1.0.0", URL: "u"}, nil)
	tick <- time.Now()
	waitEvents(t, e, 1)
}

func TestAckBeforeQuit(t *testing.T) {
	svc, e := newAppSvc(t)
	require.NoError(t, svc.AckBeforeQuit(t.Context()), "QuitAck が nil でも成功")
	n := 0
	e.Deps.QuitAck = func() { n++ }
	require.NoError(t, svc.AckBeforeQuit(t.Context()))
	require.NoError(t, svc.AckBeforeQuit(t.Context()))
	assert.Equal(t, 2, n)
}
