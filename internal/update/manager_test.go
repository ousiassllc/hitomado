package update_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/update"
	"github.com/ousiassllc/hitomado/internal/update/updatetest"
)

// fakeFetcher は Stage の代わりに、dir へファイルを書いて返す。
type fakeFetcher struct {
	dir     string
	content []byte
	err     error
	// hook は書き込み前に呼ばれる（ダウンロード中の動作のテスト用）。
	hook  func()
	mu    sync.Mutex
	calls int
}

func (f *fakeFetcher) Stage(_ context.Context, rel update.Release) (update.Staged, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.hook != nil {
		f.hook()
	}
	if f.err != nil {
		return update.Staged{}, f.err
	}
	path := filepath.Join(f.dir, update.InstallerName(rel.Version))
	if err := os.WriteFile(path, f.content, 0o600); err != nil {
		return update.Staged{}, err
	}
	return update.Staged{Version: rel.Version, Path: path, SHA256: updatetest.Sum(f.content)}, nil
}

func (f *fakeFetcher) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type launch struct {
	path string
	opts update.LaunchOptions
}

type managerEnv struct {
	dir      string
	fetcher  *fakeFetcher
	busy     bool
	launches []launch
	launchEr error
	states   []update.State
	m        *update.Manager
}

func newManagerEnv(t *testing.T, current string) *managerEnv {
	t.Helper()
	e := &managerEnv{dir: t.TempDir()}
	e.fetcher = &fakeFetcher{dir: e.dir, content: installerBody}
	e.build(t, current)
	return e
}

func (e *managerEnv) build(t *testing.T, current string) {
	t.Helper()
	e.m = update.NewManager(update.ManagerOptions{
		Fetcher: e.fetcher,
		Launch: func(p string, o update.LaunchOptions) error {
			e.launches = append(e.launches, launch{p, o})
			return e.launchEr
		},
		Busy:     func() bool { return e.busy },
		OnChange: func(s update.State) { e.states = append(e.states, s) },
		Current:  current, Dir: e.dir,
	})
	t.Cleanup(e.m.Close)
}

func release(version string) update.Release { return update.Release{Version: version} }

func TestManagerPrepareStagesAndNotifies(t *testing.T) {
	e := newManagerEnv(t, "0.1.0")
	got, err := e.m.Prepare(t.Context(), release("0.2.0"))
	require.NoError(t, err)
	assert.Equal(t, "0.2.0", got.Version)

	require.Len(t, e.states, 2)
	assert.Equal(t, "0.2.0", e.states[0].Downloading)
	assert.Nil(t, e.states[0].Staged)
	assert.Empty(t, e.states[1].Downloading)
	require.NotNil(t, e.states[1].Staged)
	assert.Equal(t, got.Path, e.states[1].Staged.Path)

	_, err = e.m.Prepare(t.Context(), release("0.2.0"))
	require.NoError(t, err)
	assert.Equal(t, 1, e.fetcher.Calls(), "用意済みの版は取り直さない")
}

func TestManagerPrepareNotApplicable(t *testing.T) {
	for label, tc := range map[string]struct{ current, latest string }{
		"dev ビルド": {update.DevVersion, "9.0.0"},
		"同じ版":     {"0.2.0", "0.2.0"},
		"古い版":     {"0.3.0", "0.2.0"},
	} {
		t.Run(label, func(t *testing.T) {
			e := newManagerEnv(t, tc.current)
			_, err := e.m.Prepare(t.Context(), release(tc.latest))
			require.ErrorIs(t, err, update.ErrNotApplicable)
			assert.Zero(t, e.fetcher.Calls(), "何もダウンロードしない")
			assert.Empty(t, e.states)
		})
	}
}

func TestManagerPrepareFailureKeepsRunning(t *testing.T) {
	e := newManagerEnv(t, "0.1.0")
	e.fetcher.err = update.ErrChecksumMismatch
	_, err := e.m.Prepare(t.Context(), release("0.2.0"))
	require.ErrorIs(t, err, update.ErrChecksumMismatch)

	st := e.m.State()
	assert.Nil(t, st.Staged)
	assert.Empty(t, st.Downloading)
	require.NotNil(t, st.Failure)
	assert.Equal(t, "0.2.0", st.Failure.Version)
	assert.Contains(t, st.Failure.Message, "一致しなかった")
	assert.NotContains(t, st.Failure.Message, "update:", "内部のエラー文面を画面に出さない")

	started, err := e.m.ApplyOnExit(true)
	require.NoError(t, err)
	assert.False(t, started)
	assert.Empty(t, e.launches, "失敗した更新は入れ替えない")

	// 次の確認でやり直せて、成功すると失敗の表示は消える。
	e.fetcher.err = nil
	_, err = e.m.Prepare(t.Context(), release("0.2.0"))
	require.NoError(t, err)
	assert.NotNil(t, e.m.State().Staged)
}

func TestManagerDiscard(t *testing.T) {
	e := newManagerEnv(t, "0.1.0")
	staged, err := e.m.Prepare(t.Context(), release("0.2.0"))
	require.NoError(t, err)
	require.NoError(t, e.m.RequestRestart())

	e.m.Discard()
	assert.NoFileExists(t, staged.Path)
	assert.Nil(t, e.m.State().Staged)
	started, err := e.m.ApplyOnExit(true)
	require.NoError(t, err)
	assert.False(t, started)
}

func TestManagerDiscardDuringDownloadDropsResult(t *testing.T) {
	e := newManagerEnv(t, "0.1.0")
	e.fetcher.hook = func() { e.m.Discard() } // ダウンロード中に自動アップデートがオフになった
	_, err := e.m.Prepare(t.Context(), release("0.2.0"))
	require.ErrorIs(t, err, update.ErrNotApplicable)
	assert.Nil(t, e.m.State().Staged)
	assert.NoFileExists(t, filepath.Join(e.dir, update.InstallerName("0.2.0")))
}

func TestManagerNewerReleaseReplacesStagedFile(t *testing.T) {
	e := newManagerEnv(t, "0.1.0")
	old, err := e.m.Prepare(t.Context(), release("0.2.0"))
	require.NoError(t, err)
	newer, err := e.m.Prepare(t.Context(), release("0.3.0"))
	require.NoError(t, err)
	assert.NoFileExists(t, old.Path, "古い版のファイルは消す")
	assert.FileExists(t, newer.Path)
	assert.Equal(t, "0.3.0", e.m.State().Staged.Version)
}

func TestManagerStartupCleansStagingDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o600))
		return p
	}
	partial := write("download-123.tmp")
	applied := write(update.InstallerName("0.2.0")) // 入れ替え済み（今の版以下）
	pending := write(update.InstallerName("0.3.0"))
	unrelated := write("keep.txt")

	update.NewManager(update.ManagerOptions{Current: "0.2.0", Dir: dir}).Close()

	assert.NoFileExists(t, partial)
	assert.NoFileExists(t, applied)
	assert.FileExists(t, pending)
	assert.FileExists(t, unrelated)
}

// 入れ替えのインストーラーを起動したのに版が上がらなかった（インストーラーの失敗）ことを、次の起動で画面へ伝え、
// 終了のたびに同じ失敗を繰り返さない（利用者の操作でだけ再試行する）。
func TestManagerDetectsIncompleteInstallOnNextStart(t *testing.T) {
	e := newManagerEnv(t, "0.1.0")
	_, err := e.m.Prepare(t.Context(), release("0.2.0"))
	require.NoError(t, err)
	started, err := e.m.ApplyOnExit(true)
	require.NoError(t, err)
	require.True(t, started)
	assert.FileExists(t, filepath.Join(e.dir, "last-attempt"))

	// 版が上がらないまま次の起動。
	e2 := &managerEnv{dir: e.dir, fetcher: e.fetcher}
	e2.build(t, "0.1.0")
	st := e2.m.State()
	require.NotNil(t, st.Failure)
	assert.Equal(t, "0.2.0", st.Failure.Version)
	assert.Contains(t, st.Failure.Message, "完了しませんでした")

	_, err = e2.m.Prepare(t.Context(), release("0.2.0")) // 取り直し（照合済みの同名ファイルがあれば再利用）
	require.NoError(t, err)
	started, err = e2.m.ApplyOnExit(true)
	require.NoError(t, err)
	assert.False(t, started, "完了しなかった版は、終了のたびには入れ替えない")
	require.NoError(t, e2.m.RequestRestart())
	started, err = e2.m.ApplyOnExit(true)
	require.NoError(t, err)
	assert.True(t, started, "利用者が選べば再試行する")
	assert.True(t, e2.launches[0].opts.Relaunch)

	// 版が上がった次の起動では記録を消す。
	e3 := &managerEnv{dir: e.dir, fetcher: e.fetcher}
	e3.build(t, "0.2.0")
	assert.Nil(t, e3.m.State().Failure)
	assert.NoFileExists(t, filepath.Join(e.dir, "last-attempt"))
}

func TestManagerPrepareAsync(t *testing.T) {
	e := newManagerEnv(t, "0.1.0")
	e.m.PrepareAsync(release("0.2.0"))
	e.m.Close() // 実行中のものの完了を待つ
	assert.NotNil(t, e.m.State().Staged)
}
