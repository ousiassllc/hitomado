package updatecore_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	appdeps "github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/updatecore"
	"github.com/ousiassllc/hitomado/internal/capability"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/store"
	"github.com/ousiassllc/hitomado/internal/update"
	"github.com/ousiassllc/hitomado/internal/update/updatetest"
)

var fakeInstaller = []byte("MZ fake installer for hitomado")

// autoUpdateEnv は偽の GitHub Release（httptest）と、実際の Stager・Manager を組んだ環境。
// インストーラーの起動は実行せず、呼び出しを記録するだけ（launches）。
type autoUpdateEnv struct {
	*apptest.Env
	Svc      *app.AppService
	Server   *updatetest.Server
	Dir      string
	Busy     *appdeps.Busy
	launches []update.LaunchOptions
	paths    []string
	quits    int
}

func newAutoUpdateEnv(t *testing.T, latest string) *autoUpdateEnv {
	t.Helper()
	e := apptest.NewEnv(t)
	srv := updatetest.NewServer(t, latest, fakeInstaller)
	a := &autoUpdateEnv{Env: e, Server: srv, Dir: filepath.Join(t.TempDir(), "updates"), Busy: &appdeps.Busy{}}
	e.Deps.Update = srv.Checker()
	e.Deps.Busy = a.Busy
	e.Deps.RequestQuit = func() { a.quits++ }
	e.Deps.Updates = update.NewManager(update.ManagerOptions{
		Fetcher: srv.Stager(a.Dir),
		Launch: func(path string, o update.LaunchOptions) error {
			a.paths = append(a.paths, path)
			a.launches = append(a.launches, o)
			return nil
		},
		Busy:     a.Busy.Active,
		OnChange: func(st update.State) { e.Events.Emit(events.UpdateStatus, updatecore.StatusPayload(st)) },
		Current:  e.Deps.Version, Dir: a.Dir,
	})
	t.Cleanup(e.Deps.Updates.Close)
	a.Svc = app.NewAppService(e.Deps)
	return a
}

func requireCode(t *testing.T, want string, err error) {
	t.Helper()
	require.Error(t, err)
	var ae *apperr.Error
	require.True(t, errors.As(err, &ae), "apperr.Error ではない: %v", err)
	require.Equal(t, want, ae.Code)
}

func (a *autoUpdateEnv) applyOnExit(ctx context.Context) bool {
	return updatecore.New((*appdeps.Deps)(a.Deps)).ApplyOnExit(ctx)
}

// waitStage は update:status が stage になるまで待つ（上限つき）。
func (a *autoUpdateEnv) waitStage(t *testing.T, stage string) events.UpdateStatusPayload {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for n := 1; ; n++ {
		got := a.Events.Wait(ctx, events.UpdateStatus, n)
		require.NotNil(t, got, "update:status が %s にならない", stage)
		if p := got[n-1].Payload.(events.UpdateStatusPayload); p.Stage == stage {
			return p
		}
	}
}

func (a *autoUpdateEnv) files(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(a.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// 新しい版の検出 → ダウンロード → ハッシュの照合 → 入れ替えの指示（終了時）まで。
func TestAutoUpdateDetectDownloadVerifyApply(t *testing.T) {
	a := newAutoUpdateEnv(t, "0.2.0")
	ctx := t.Context()

	info, err := a.Svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	assert.True(t, info.Available)
	assert.Equal(t, "0.2.0", info.Version)

	ready := a.waitStage(t, events.UpdateStageReady)
	assert.Equal(t, "0.2.0", ready.Version)
	assert.Empty(t, ready.Message)
	st, err := a.Svc.GetUpdateStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.UpdateStageReady, st.Stage)
	assert.Equal(t, []string{"hitomado-v0.2.0-windows-amd64-setup.exe"}, a.files(t))
	assert.Empty(t, a.launches, "ダウンロードしただけでは入れ替えない")

	// 次の終了で入れ替える（起動し直しは付けない）。
	assert.True(t, a.applyOnExit(ctx))
	require.Len(t, a.launches, 1)
	assert.False(t, a.launches[0].Relaunch)
	assert.Equal(t, filepath.Join(a.Dir, "hitomado-v0.2.0-windows-amd64-setup.exe"), a.paths[0])
}

func TestAutoUpdateRestartToUpdate(t *testing.T) {
	a := newAutoUpdateEnv(t, "0.2.0")
	ctx := t.Context()
	requireCode(t, apperr.Validation, a.Svc.RestartToUpdate(ctx)) // まだ用意できていない
	assert.Zero(t, a.quits)

	_, err := a.Svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	a.waitStage(t, events.UpdateStageReady)

	require.NoError(t, a.Svc.RestartToUpdate(ctx))
	assert.Equal(t, 1, a.quits, "終了を要求する")
	assert.Empty(t, a.launches, "入れ替えは終了処理の中で行う")
	assert.True(t, a.applyOnExit(ctx))
	require.Len(t, a.launches, 1)
	assert.True(t, a.launches[0].Relaunch, "再起動して更新は、インストール後に起動し直す")
}

func TestAutoUpdateRestartToUpdateRefusedWhileBusy(t *testing.T) {
	a := newAutoUpdateEnv(t, "0.2.0")
	ctx := t.Context()
	_, err := a.Svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	a.waitStage(t, events.UpdateStageReady)

	end := a.Busy.Begin() // 送信中・下書きの保存中
	requireCode(t, apperr.Validation, a.Svc.RestartToUpdate(ctx))
	assert.Zero(t, a.quits)
	assert.False(t, a.applyOnExit(ctx), "送信中は終了時にも入れ替えない")
	assert.Empty(t, a.launches)

	end()
	assert.True(t, a.applyOnExit(ctx), "終わっていれば次の終了で入れ替える")
}

// ハッシュが合わなければ使わず（ファイルを捨て）、入れ替えない。今の版のまま使え、画面へ知らせる。
func TestAutoUpdateChecksumMismatchDiscards(t *testing.T) {
	a := newAutoUpdateEnv(t, "0.2.0")
	a.Server.Sums = updatetest.Sum([]byte("other")) + "  " + update.InstallerName("0.2.0") + "\n"
	ctx := t.Context()

	_, err := a.Svc.CheckForUpdate(ctx)
	require.NoError(t, err, "確認自体は成功する（新版の通知は出る）")
	failed := a.waitStage(t, events.UpdateStageFailed)
	assert.Equal(t, "0.2.0", failed.Version)
	assert.Contains(t, failed.Message, "一致しなかった")
	assert.NotContains(t, failed.Message, "http", "URL を画面に出さない")
	assert.Empty(t, a.files(t), "合わないファイルは捨てる")

	assert.False(t, a.applyOnExit(ctx))
	requireCode(t, apperr.Validation, a.Svc.RestartToUpdate(ctx))
	assert.Empty(t, a.launches, "入れ替えを指示しない")
}

func TestAutoUpdateDoesNothingWhenNotApplicable(t *testing.T) {
	tests := []struct {
		name          string
		wantAvailable bool // 新版の通知（リンク）が出るか
		setup         func(*testing.T, *autoUpdateEnv)
	}{
		{"自動アップデートがオフ", true, func(t *testing.T, a *autoUpdateEnv) {
			require.NoError(t, a.Svc.SetAutoUpdateEnabled(t.Context(), false))
		}},
		{"dev ビルド（0.0.0）", false, func(_ *testing.T, a *autoUpdateEnv) { a.Deps.Version = update.DevVersion }},
		{"Android（リンクを出すだけ。入れ替えの仕組みが無い）", true, func(_ *testing.T, a *autoUpdateEnv) {
			a.Deps.Capabilities = capability.Android()
			a.Deps.Updates = nil
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newAutoUpdateEnv(t, "0.2.0")
			tc.setup(t, a)
			svc := app.NewAppService(a.Deps)

			info, err := svc.CheckForUpdate(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tc.wantAvailable, info.Available)
			if a.Deps.Updates != nil {
				a.Deps.Updates.Close() // バックグラウンドのダウンロードがあれば終わりを待つ
			}
			assert.Zero(t, a.Server.TotalHits(), "リンクの確認以外の通信（ダウンロード）をしない")
			assert.Empty(t, a.files(t))
			assert.False(t, updatecore.New((*appdeps.Deps)(a.Deps)).ApplyOnExit(t.Context()))
			assert.Empty(t, a.launches)
		})
	}
}

func TestAutoUpdateSettingRoundTripAndDiscard(t *testing.T) {
	a := newAutoUpdateEnv(t, "0.2.0")
	ctx := t.Context()
	info, err := a.Svc.GetAppInfo(ctx)
	require.NoError(t, err)
	assert.True(t, info.AutoUpdateSupported)
	assert.True(t, info.AutoUpdateEnabled, "既定はオン")

	_, err = a.Svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	a.waitStage(t, events.UpdateStageReady)

	require.NoError(t, a.Svc.SetAutoUpdateEnabled(ctx, false))
	info, _ = a.Svc.GetAppInfo(ctx)
	assert.False(t, info.AutoUpdateEnabled)
	assert.Empty(t, a.files(t), "オフにすると用意済みのファイルも捨てる")
	st, _ := a.Svc.GetUpdateStatus(ctx)
	assert.Equal(t, events.UpdateStageNone, st.Stage)
	assert.False(t, a.applyOnExit(ctx))

	require.NoError(t, a.Svc.SetAutoUpdateEnabled(ctx, true))
	info, _ = a.Svc.GetAppInfo(ctx)
	assert.True(t, info.AutoUpdateEnabled)
	raw, ok, err := a.Store.GetSetting(ctx, store.SettingAutoUpdate)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "true", raw)
}

// 更新確認そのものをオフにすると、用意済みの更新も入れ替えない。
func TestUpdateCheckOffDiscardsStagedUpdate(t *testing.T) {
	a := newAutoUpdateEnv(t, "0.2.0")
	ctx := t.Context()
	_, err := a.Svc.CheckForUpdate(ctx)
	require.NoError(t, err)
	a.waitStage(t, events.UpdateStageReady)

	require.NoError(t, a.Svc.SetUpdateCheckEnabled(ctx, false))
	assert.Empty(t, a.files(t))
	assert.False(t, a.applyOnExit(ctx))
}

func TestAutoUpdateUnsupportedBuild(t *testing.T) {
	e := apptest.NewEnv(t) // Updates が nil（Android・開発用の Linux）
	svc := app.NewAppService(e.Deps)
	ctx := t.Context()
	info, err := svc.GetAppInfo(ctx)
	require.NoError(t, err)
	assert.False(t, info.AutoUpdateSupported)
	assert.False(t, info.AutoUpdateEnabled)
	requireCode(t, apperr.Unsupported, svc.SetAutoUpdateEnabled(ctx, true))
	requireCode(t, apperr.Unsupported, svc.RestartToUpdate(ctx))
	st, err := svc.GetUpdateStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.UpdateStageNone, st.Stage)
	assert.False(t, updatecore.New((*appdeps.Deps)(e.Deps)).ApplyOnExit(ctx))
}

// UpdateLoop（起動時・24 時間ごと）の確認でも、自動アップデートはダウンロードまで進む。
func TestUpdateLoopDownloadsWhenAutoUpdateEnabled(t *testing.T) {
	a := newAutoUpdateEnv(t, "0.2.0")
	ticks := make(chan time.Time)
	a.Deps.UpdateTicker = func() (<-chan time.Time, func()) { return ticks, func() {} }
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); updatecore.NewLoop((*appdeps.Deps)(a.Deps)).Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })

	a.waitStage(t, events.UpdateStageReady)
	assert.True(t, a.applyOnExit(t.Context()))
}
