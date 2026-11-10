package apptests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/capability"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
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
	assert.Equal(t, dto.AppInfo{Version: "0.1.0", UpdateCheckEnabled: true, DataDir: e.Deps.DataDir, SendConfirmEnabled: true}, info)

	require.NoError(t, svc.SetUpdateCheckEnabled(ctx, false))
	info, _ = svc.GetAppInfo(ctx)
	assert.False(t, info.UpdateCheckEnabled)
	require.NoError(t, svc.SetUpdateCheckEnabled(ctx, true))
	info, _ = svc.GetAppInfo(ctx)
	assert.True(t, info.UpdateCheckEnabled)
}

func TestSendConfirmSetting(t *testing.T) {
	svc, e := newAppSvc(t)
	ctx := t.Context()
	info, err := svc.GetAppInfo(ctx)
	require.NoError(t, err)
	assert.True(t, info.SendConfirmEnabled, "既定は確認画面を出す")

	require.NoError(t, svc.SetSendConfirmEnabled(ctx, false))
	info, _ = svc.GetAppInfo(ctx)
	assert.False(t, info.SendConfirmEnabled)
	require.NoError(t, svc.SetSendConfirmEnabled(ctx, true))
	info, _ = svc.GetAppInfo(ctx)
	assert.True(t, info.SendConfirmEnabled)

	require.NoError(t, e.Store.SetSetting(ctx, store.SettingSendConfirm, `"garbage"`))
	info, _ = svc.GetAppInfo(ctx)
	assert.True(t, info.SendConfirmEnabled, "壊れた保存値は既定（オン）に戻す")
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

// dev・ローカルビルド（版 0.0.0）は更新確認をしない（通信せず・新版の状態も作らない）。配布物（X.Y.Z）は従来どおり確認する。
func TestCheckForUpdateSkipsDevBuild(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		wantCalls     int
		want          dto.UpdateInfo
	}{
		{"dev 0.0.0", "0.0.0", 0, dto.UpdateInfo{}},
		{"配布物 X.Y.Z", "1.2.3", 1, dto.UpdateInfo{Available: true, Version: "9.9.9", URL: "https://example.com/r"}},
		{"CI の 0.99.x", "0.99.2", 1, dto.UpdateInfo{Available: true, Version: "9.9.9", URL: "https://example.com/r"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, e := newAppSvc(t)
			e.Deps.Version = tc.version
			e.Update.Release = update.Release{Version: "9.9.9", URL: "https://example.com/r"}

			info, err := svc.CheckForUpdate(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tc.want, info)
			assert.Equal(t, tc.wantCalls, e.Update.CallCount())
		})
	}
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

// アプリ内の更新確認を持たない機能構成（ゼロ値）では、PC で保存した「有効」の設定や dev でない版でも、確認せず・通信せず・無効を返す（#89）。
func TestUpdateCheckDisabledWhenUnsupported(t *testing.T) {
	svc, e := newAppSvc(t)
	ctx := t.Context()
	e.Deps.Capabilities = capability.Capabilities{}
	require.NoError(t, e.Store.SetSetting(ctx, store.SettingUpdateCheck, "true"))
	e.Update.Release = update.Release{Version: "9.9.9", URL: "https://example.com/r"}

	info, err := svc.GetAppInfo(ctx)
	require.NoError(t, err)
	assert.False(t, info.UpdateCheckEnabled)

	got, err := svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	assert.Equal(t, dto.UpdateInfo{}, got)
	assert.Zero(t, e.Update.CallCount(), "通信しない")

	err = svc.SetUpdateCheckEnabled(ctx, true)
	var ae *apperr.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperr.Unsupported, ae.Code)
}

// Android は新しい版のお知らせ（ダウンロードページのリンク）だけを持つ。確認はでき、ダウンロード・入れ替えの仕組みは無い。
func TestUpdateCheckOnAndroidOnlyNotifies(t *testing.T) {
	svc, e := newAppSvc(t)
	ctx := t.Context()
	e.Deps.Capabilities = capability.Android()
	e.Update.Release = update.Release{Version: "9.9.9", URL: "https://example.com/r"}

	info, err := svc.GetAppInfo(ctx)
	require.NoError(t, err)
	assert.True(t, info.UpdateCheckEnabled, "既定はオン")
	assert.False(t, info.AutoUpdateSupported)
	assert.False(t, info.AutoUpdateEnabled)

	got, err := svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	assert.Equal(t, dto.UpdateInfo{Available: true, Version: "9.9.9", URL: "https://example.com/r"}, got)

	require.NoError(t, svc.SetUpdateCheckEnabled(ctx, false))
	got, err = svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	assert.Equal(t, dto.UpdateInfo{}, got, "オフなら確認しない")
}
