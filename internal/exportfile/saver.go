package exportfile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Native は Android の Java 側（DocumentExporter）への呼び出し。実装は platform の savefile_android.go（JNI）。
// どれも結果を待たずに戻る。選択・保存の結果は Saver.Deliver で届く。
type Native interface {
	// TempDir は一時ファイルを置く、アプリの cache 配下の専用ディレクトリ（Java が作り、Java が書き込み元として許す唯一の場所）。
	TempDir() (string, error)
	// Begin は保存先の選択（ACTION_CREATE_DOCUMENT）を始める。別の保存が進行中なら ErrBusy。
	Begin(id int, name, mime string) error
	// Commit は path（TempDir の中）の内容を、id で選ばれた保存先へコピーさせる。
	Commit(id int, path string) error
	// Abort は id の保存を取りやめ、選ばれた保存先に作られた空のファイルを消させる。
	Abort(id int) error
}

// Saver が返すエラー。
var (
	// ErrBusy は別の保存が進行中。
	ErrBusy = errors.New("another save is in progress")
	// ErrTimeout は Java からの結果が時間内に届かなかった。
	ErrTimeout = errors.New("timed out waiting for the document provider")
)

// Saver は Android の保存（選択 → 一時ファイルへ書く → Java がコピー）を Go 側から進める。
// 結果は非同期（Activity の結果）で届くので、保存ごとの id で待ち合わせる。
//
// プロセスが保存の途中で終わった場合、Go 側の待ちも一時ファイルも一緒に消える（結果は捨てる）。
// 次回の最初の Pick が一時ディレクトリを空にして取りこぼしを片付ける。選択だけ済んで空のまま残る保存先は、
// Java が（Uri が分かる範囲で）消す。
type Saver struct {
	native        Native
	pickTimeout   time.Duration // 利用者が保存先を選ぶ待ち
	commitTimeout time.Duration // コピーの待ち

	mu      sync.Mutex
	nextID  int
	active  bool // Pick から Close までの間 true（同時に 1 件だけ）
	waiters map[int]chan Result
	swept   bool // 一時ディレクトリの取りこぼしを片付け済みか
}

// NewSaver は native を使う Saver を返す。
func NewSaver(native Native) *Saver {
	return &Saver{
		native: native, pickTimeout: 10 * time.Minute, commitTimeout: 5 * time.Minute,
		waiters: map[int]chan Result{},
	}
}

// Deliver は Java から届いた結果を、待っている保存へ渡す。待っている保存が無ければ捨てる。
func (s *Saver) Deliver(r Result) {
	s.mu.Lock()
	ch := s.waiters[r.ID]
	s.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- r:
	default: // 1 件ずつ受け取るので、溢れたら（重複通知）捨てる
	}
}

// Pick は保存先を選ばせる。キャンセルは (nil, false, nil)。選ばれたら、内容の書き込み先の一時ファイルを持つ Target を返す
// （呼び出し側は Close を必ず呼ぶ）。
func (s *Saver) Pick(ctx context.Context, suggestedName string) (Target, bool, error) {
	dir, err := s.prepareTempDir()
	if err != nil {
		return nil, false, fmt.Errorf("prepare temp dir: %w", err)
	}
	id, ch, err := s.start()
	if err != nil {
		return nil, false, err
	}
	name := DisplayName(suggestedName)
	if err := s.native.Begin(id, name, MIMEForName(name)); err != nil {
		s.finish(id)
		return nil, false, fmt.Errorf("begin save: %w", err)
	}
	r, err := s.wait(ctx, ch, s.pickTimeout)
	switch {
	case err != nil: // 待ちの打ち切り。Java 側の選択状態も閉じる
		_ = s.native.Abort(id)
		s.finish(id)
		return nil, false, err
	case r.Status == StatusCancelled:
		s.finish(id)
		return nil, false, nil
	case r.Status != StatusPicked:
		s.finish(id)
		return nil, false, fmt.Errorf("pick document: %s", r.Error)
	}
	return &docTarget{s: s, id: id, name: name, path: filepath.Join(dir, "save-"+strconv.Itoa(id)), ch: ch}, true, nil
}

// prepareTempDir は一時ディレクトリを返す。最初の 1 回だけ、前回のプロセスが残した中身を空にする。
func (s *Saver) prepareTempDir() (string, error) {
	dir, err := s.native.TempDir()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	sweep := !s.swept && !s.active
	s.swept = s.swept || sweep
	s.mu.Unlock()
	if sweep {
		_ = os.RemoveAll(dir)
	}
	return dir, os.MkdirAll(dir, 0o700)
}

func (s *Saver) start() (int, chan Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return 0, nil, ErrBusy
	}
	s.active = true
	s.nextID++
	ch := make(chan Result, 1)
	s.waiters[s.nextID] = ch
	return s.nextID, ch, nil
}

func (s *Saver) finish(id int) {
	s.mu.Lock()
	delete(s.waiters, id)
	s.active = false
	s.mu.Unlock()
}

func (s *Saver) wait(ctx context.Context, ch <-chan Result, timeout time.Duration) (Result, error) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case r := <-ch:
		return r, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-t.C:
		return Result{}, ErrTimeout
	}
}

// docTarget は選ばれた保存先（Android の Uri）への Target。内容はいったん一時ファイル path へ書く。
type docTarget struct {
	s    *Saver
	id   int
	name string // 利用者に見せる名前の既定（Java が実際の名前を返さなかったとき）
	path string
	ch   chan Result

	mu     sync.Mutex
	closed bool
}

func (t *docTarget) Path() string { return t.path }

// Commit は一時ファイルを保存先へコピーさせ、終わるまで待つ。待ちの打ち切り（ctx・時間切れ）では保存を取りやめる。
func (t *docTarget) Commit(ctx context.Context) (string, error) {
	if err := t.s.native.Commit(t.id, t.path); err != nil {
		return "", fmt.Errorf("commit save: %w", err)
	}
	r, err := t.s.wait(ctx, t.ch, t.s.commitTimeout)
	if err != nil {
		return "", err // Close が Abort する
	}
	if r.Status != StatusSaved {
		return "", fmt.Errorf("save document: %s", r.Error)
	}
	t.release(false)
	if r.Name != "" {
		return r.Name, nil
	}
	return t.name, nil
}

// Close は未確定なら Java の選択状態（と空の保存先）を片付け、一時ファイルを消す。
func (t *docTarget) Close() { t.release(true) }

func (t *docTarget) release(abort bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	if abort {
		_ = t.s.native.Abort(t.id)
	}
	_ = os.Remove(t.path)
	t.s.finish(t.id)
}
