package exportfile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeNative は Java（DocumentExporter）の代わり。Begin／Commit の応答を onBegin／onCommit で決め、
// 結果は Saver.Deliver で返す（実機では Activity の結果が非同期に届く）。
type fakeNative struct {
	mu      sync.Mutex
	s       *Saver
	dir     string
	dest    map[int][]byte // 保存先へコピーされた内容（id ごと）
	begun   []beginCall
	aborted []int

	beginErr  error
	commitErr error
	onBegin   func(id int) *Result // nil なら結果を返さない（利用者がまだ選んでいない）
	onCommit  func(id int, path string) *Result
}

type beginCall struct {
	ID         int
	Name, MIME string
}

func newFakeNative(t *testing.T) (*fakeNative, *Saver) {
	t.Helper()
	f := &fakeNative{dir: filepath.Join(t.TempDir(), "exports"), dest: map[int][]byte{}}
	// 既定: 選ばれ、コピーが成功する。
	f.onBegin = func(id int) *Result { return &Result{ID: id, Status: StatusPicked} }
	f.onCommit = func(id int, path string) *Result {
		b, err := os.ReadFile(path) //nolint:gosec // テストの一時ディレクトリ内のパス
		if err != nil {
			return &Result{ID: id, Status: StatusFailed, Error: "read_failed"}
		}
		f.dest[id] = b
		return &Result{ID: id, Status: StatusSaved, Name: "chosen.pdf"}
	}
	f.s = NewSaver(f)
	f.s.pickTimeout, f.s.commitTimeout = time.Second, time.Second
	return f, f.s
}

func (f *fakeNative) TempDir() (string, error) { return f.dir, nil }

func (f *fakeNative) Begin(id int, name, mime string) error {
	f.mu.Lock()
	f.begun = append(f.begun, beginCall{id, name, mime})
	f.mu.Unlock()
	if f.beginErr != nil {
		return f.beginErr
	}
	if r := f.onBegin(id); r != nil {
		f.s.Deliver(*r)
	}
	return nil
}

func (f *fakeNative) Commit(id int, path string) error {
	if f.commitErr != nil {
		return f.commitErr
	}
	if r := f.onCommit(id, path); r != nil {
		f.s.Deliver(*r)
	}
	return nil
}

func (f *fakeNative) Abort(id int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aborted = append(f.aborted, id)
	return nil
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := []string{}
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func TestSaverPickWriteCommit(t *testing.T) {
	f, s := newFakeNative(t)
	tg, ok, err := s.Pick(t.Context(), "re:port.pdf")
	require.NoError(t, err)
	require.True(t, ok)
	defer tg.Close()

	assert.Equal(t, []beginCall{{1, "re_port.pdf", "application/pdf"}}, f.begun, "名前は安全にし、MIME は拡張子から")
	assert.Equal(t, f.dir, filepath.Dir(tg.Path()), "一時ファイルは Java の指定したディレクトリの中")
	require.NoError(t, os.WriteFile(tg.Path(), []byte("DATA"), 0o600))

	loc, err := tg.Commit(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "chosen.pdf", loc, "Java が返した保存先の名前")
	assert.Equal(t, []byte("DATA"), f.dest[1])
	assert.Empty(t, entries(t, f.dir), "コピー後に一時ファイルを残さない")
	assert.Empty(t, f.aborted, "確定した保存は Abort しない")

	tg.Close() // 確定後の Close は安全（何もしない）
	assert.Empty(t, f.aborted)
	_, _, err = s.Pick(t.Context(), "next.zip")
	assert.NoError(t, err, "確定したら次の保存を始められる")
}

func TestSaverCommitFallsBackToRequestedName(t *testing.T) {
	f, s := newFakeNative(t)
	f.onCommit = func(id int, _ string) *Result { return &Result{ID: id, Status: StatusSaved} }
	tg, _, err := s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	loc, err := tg.Commit(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "a.pdf", loc)
}

func TestSaverCancelled(t *testing.T) {
	f, s := newFakeNative(t)
	f.onBegin = func(id int) *Result { return &Result{ID: id, Status: StatusCancelled} }
	tg, ok, err := s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	assert.False(t, ok, "キャンセルは ok=false でエラーにしない")
	assert.Nil(t, tg)
	assert.Empty(t, f.aborted, "キャンセル済みなら Java へ Abort は要らない")

	f.onBegin = func(id int) *Result { return &Result{ID: id, Status: StatusPicked} }
	_, ok, err = s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	assert.True(t, ok, "キャンセルの後でも再び保存できる")
}

func TestSaverPickErrors(t *testing.T) {
	f, s := newFakeNative(t)

	f.beginErr = &BridgeError{Code: "unavailable"}
	_, _, err := s.Pick(t.Context(), "a.pdf")
	var be *BridgeError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, "unavailable", be.Code)
	f.beginErr = nil

	f.onBegin = func(id int) *Result { return &Result{ID: id, Status: StatusFailed, Error: "no_picker"} }
	_, _, err = s.Pick(t.Context(), "a.pdf")
	assert.ErrorContains(t, err, "no_picker")

	f.onBegin = func(id int) *Result { return &Result{ID: id, Status: StatusPicked} }
	_, ok, err := s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	assert.True(t, ok, "失敗の後でも busy のまま残らない")
}

func TestSaverBusyWhileTargetOpen(t *testing.T) {
	_, s := newFakeNative(t)
	tg, _, err := s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	_, _, err = s.Pick(t.Context(), "b.pdf")
	assert.ErrorIs(t, err, ErrBusy)
	tg.Close()
	_, ok, err := s.Pick(t.Context(), "b.pdf")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestSaverCloseWithoutCommitAbortsAndRemovesTemp(t *testing.T) {
	f, s := newFakeNative(t)
	tg, _, err := s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tg.Path(), []byte("x"), 0o600))

	tg.Close()
	assert.Equal(t, []int{1}, f.aborted, "書き込みに失敗して Commit しなかった保存は Java へ取りやめを伝える（空の保存先を消させる）")
	assert.Empty(t, entries(t, f.dir))
	tg.Close()
	assert.Equal(t, []int{1}, f.aborted, "Close は何度呼んでも 1 回だけ")
}

func TestSaverPickWaitsUntilDelivered(t *testing.T) {
	f, s := newFakeNative(t)
	f.onBegin = func(int) *Result { return nil } // 利用者はまだ選んでいない
	type res struct {
		ok  bool
		err error
	}
	done := make(chan res, 1)
	go func() {
		tg, ok, err := s.Pick(t.Context(), "a.pdf")
		if tg != nil {
			tg.Close()
		}
		done <- res{ok, err}
	}()
	select {
	case <-done:
		t.Fatal("結果が届く前に戻った")
	case <-time.After(50 * time.Millisecond): // 「まだ戻らない」ことの確認なので有限の待ちで足りる
	}
	s.Deliver(Result{ID: 1, Status: StatusPicked})
	select {
	case r := <-done:
		assert.NoError(t, r.err)
		assert.True(t, r.ok)
	case <-time.After(5 * time.Second):
		t.Fatal("結果を届けても戻らない")
	}
}

func TestSaverPickContextCancelAborts(t *testing.T) {
	f, s := newFakeNative(t)
	f.onBegin = func(int) *Result { return nil }
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, ok, err := s.Pick(ctx, "a.pdf")
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, ok)
	assert.Equal(t, []int{1}, f.aborted)
	_, _, err = s.Pick(t.Context(), "a.pdf")
	assert.NotErrorIs(t, err, ErrBusy, "打ち切りの後は busy のまま残らない")
}

func TestSaverPickTimeout(t *testing.T) {
	f, s := newFakeNative(t)
	f.onBegin = func(int) *Result { return nil }
	s.pickTimeout = 10 * time.Millisecond
	_, _, err := s.Pick(t.Context(), "a.pdf")
	assert.ErrorIs(t, err, ErrTimeout)
	assert.Equal(t, []int{1}, f.aborted)
}

func TestSaverCommitFailures(t *testing.T) {
	f, s := newFakeNative(t)
	f.onCommit = func(id int, _ string) *Result { return &Result{ID: id, Status: StatusFailed, Error: "write_failed"} }
	tg, _, err := s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tg.Path(), []byte("x"), 0o600))
	_, err = tg.Commit(t.Context())
	assert.ErrorContains(t, err, "write_failed")
	tg.Close()
	assert.Empty(t, entries(t, f.dir), "失敗しても一時ファイルを残さない")

	f.commitErr = &BridgeError{Code: "bad_path"}
	tg, _, err = s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	_, err = tg.Commit(t.Context())
	var be *BridgeError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, "bad_path", be.Code)
	tg.Close()

	// コピーの結果が届かない（Java が応答しない）。
	f.commitErr = nil
	f.onCommit = func(int, string) *Result { return nil }
	s.commitTimeout = 10 * time.Millisecond
	tg, _, err = s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	_, err = tg.Commit(t.Context())
	assert.True(t, errors.Is(err, ErrTimeout))
	tg.Close()
	assert.NotEmpty(t, f.aborted, "時間切れの保存は Close で取りやめる")
}

func TestSaverSweepsLeftoversOnce(t *testing.T) {
	f, s := newFakeNative(t)
	require.NoError(t, os.MkdirAll(f.dir, 0o700))
	stale := filepath.Join(f.dir, "save-9")
	require.NoError(t, os.WriteFile(stale, []byte("old"), 0o600))

	tg, _, err := s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	_, statErr := os.Stat(stale)
	assert.True(t, os.IsNotExist(statErr), "前回のプロセスが残した一時ファイルは最初の保存で片付ける")

	// 進行中の保存の一時ファイルは、2 回目以降の Pick（busy）で消さない。
	require.NoError(t, os.WriteFile(tg.Path(), []byte("live"), 0o600))
	_, _, err = s.Pick(t.Context(), "b.pdf")
	require.ErrorIs(t, err, ErrBusy)
	assert.FileExists(t, tg.Path())
	tg.Close()
}

func TestDeliverIgnoresUnknownID(t *testing.T) {
	_, s := newFakeNative(t)
	s.Deliver(Result{ID: 99, Status: StatusSaved}) // 待ちが無くても止まらない・溜めない
	tg, ok, err := s.Pick(t.Context(), "a.pdf")
	require.NoError(t, err)
	assert.True(t, ok)
	tg.Close()
}
