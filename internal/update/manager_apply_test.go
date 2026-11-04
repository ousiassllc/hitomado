package update_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/update"
)

func TestManagerApplyOnExit(t *testing.T) {
	prepared := func(t *testing.T) *managerEnv {
		e := newManagerEnv(t, "0.1.0")
		_, err := e.m.Prepare(t.Context(), release("0.2.0"))
		require.NoError(t, err)
		return e
	}

	t.Run("自動アップデートが有効なら次の終了で、起動し直しなしで入れ替える", func(t *testing.T) {
		e := prepared(t)
		started, err := e.m.ApplyOnExit(true)
		require.NoError(t, err)
		assert.True(t, started)
		require.Len(t, e.launches, 1)
		assert.Equal(t, filepath.Join(e.dir, "hitomado-v0.2.0-windows-amd64-setup.exe"), e.launches[0].path)
		assert.False(t, e.launches[0].opts.Relaunch)
	})
	t.Run("自動アップデートが無効で「再起動して更新」も押していなければ入れ替えない", func(t *testing.T) {
		e := prepared(t)
		started, err := e.m.ApplyOnExit(false)
		require.NoError(t, err)
		assert.False(t, started)
		assert.Empty(t, e.launches)
	})
	t.Run("「再起動して更新」ならインストール後に起動し直す", func(t *testing.T) {
		e := prepared(t)
		require.NoError(t, e.m.RequestRestart())
		started, err := e.m.ApplyOnExit(true)
		require.NoError(t, err)
		assert.True(t, started)
		require.Len(t, e.launches, 1)
		assert.True(t, e.launches[0].opts.Relaunch)
	})
	t.Run("更新が無ければ何もしない", func(t *testing.T) {
		e := newManagerEnv(t, "0.1.0")
		started, err := e.m.ApplyOnExit(true)
		require.NoError(t, err)
		assert.False(t, started)
		assert.Empty(t, e.launches)
		require.ErrorIs(t, e.m.RequestRestart(), update.ErrNoUpdate)
	})
	t.Run("送信中・下書きの保存中は入れ替えず、終わってからなら入れ替える", func(t *testing.T) {
		e := prepared(t)
		e.busy = true
		require.ErrorIs(t, e.m.RequestRestart(), update.ErrBusy)
		started, err := e.m.ApplyOnExit(true)
		require.ErrorIs(t, err, update.ErrBusy)
		assert.False(t, started)
		assert.Empty(t, e.launches)
		assert.NotNil(t, e.m.State().Staged, "ファイルは残る（次の終了に回す）")

		e.busy = false
		started, err = e.m.ApplyOnExit(true)
		require.NoError(t, err)
		assert.True(t, started)
	})
	t.Run("入れ替えの直前に照合し直し、合わなければ捨てる", func(t *testing.T) {
		e := prepared(t)
		st := e.m.State().Staged
		require.NoError(t, os.WriteFile(st.Path, []byte("tampered"), 0o600))
		started, err := e.m.ApplyOnExit(true)
		require.ErrorIs(t, err, update.ErrChecksumMismatch)
		assert.False(t, started)
		assert.Empty(t, e.launches)
		assert.NoFileExists(t, st.Path)
		assert.Nil(t, e.m.State().Staged)
	})
	t.Run("ファイルが消えていても入れ替えない", func(t *testing.T) {
		e := prepared(t)
		require.NoError(t, os.Remove(e.m.State().Staged.Path))
		started, err := e.m.ApplyOnExit(true)
		require.Error(t, err)
		assert.False(t, started)
		assert.Empty(t, e.launches)
	})
	t.Run("インストーラーを起動できなければ失敗を返す（今の版のまま終了する）", func(t *testing.T) {
		e := prepared(t)
		e.launchEr = errors.New("boom")
		started, err := e.m.ApplyOnExit(true)
		require.Error(t, err)
		assert.False(t, started)
		assert.Contains(t, update.UserMessage(err), "インストーラーを起動できませんでした")
		assert.NoFileExists(t, filepath.Join(e.dir, "last-attempt"), "起動できなかったなら試行の記録を残さない")
	})
}
