package update

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// attemptFile は入れ替えのためにインストーラーを起動した版を記録するファイル（Dir 直下）。
// 次の起動でも版が上がっていなければ、インストーラーが完了しなかったと分かる。
const attemptFile = "last-attempt"

var (
	// ErrNoUpdate は入れ替え待ちの更新が無いことを表す。
	ErrNoUpdate = errors.New("update: 入れ替え待ちの更新がありません")
	// ErrBusy は送信中・下書きの保存中で、いまは入れ替えを始められないことを表す。
	ErrBusy = errors.New("update: 送信または下書きの保存の途中のため、入れ替えられません")
	// ErrNotApplicable は dev ビルド、または今の版より新しくない版を入れ替えの対象にしようとしたことを表す。
	ErrNotApplicable = errors.New("update: 入れ替えの対象ではない版です")
	// ErrUnsupported はこの OS では更新ファイルの入れ替えを行わないことを表す。
	ErrUnsupported = errors.New("update: この OS では自動で入れ替えられません")
	// errLaunch はインストーラーを起動できなかったことを表す。
	errLaunch = errors.New("update: インストーラーを起動できません")
)

// LaunchOptions はインストーラーの起動の指定。
type LaunchOptions struct {
	// Relaunch はインストール後にアプリを起動し直すか（「再起動して更新」）。終了時の入れ替えでは起動し直さない。
	Relaunch bool
}

// Launcher は installer を、起動したプロセスが終わっても生き続ける別プロセスとして起動する。
// 起動だけを行い、完了は待たない（アプリが終了してからインストーラーが入れ替える）。
type Launcher func(installer string, opts LaunchOptions) error

// Fetcher は Release の更新ファイルをダウンロードして照合する（*Stager が実装する）。
type Fetcher interface {
	Stage(ctx context.Context, rel Release) (Staged, error)
}

// Failure は直近の失敗。画面へ伝える文面を持つ（機密を含めない）。
type Failure struct {
	Version string
	Message string
	// interrupted は「前回の入れ替えが完了しなかった」の失敗（起動時に設定し、noAuto と対）。ダウンロードの成功では消さない。
	interrupted bool
}

// State は Manager の状態のスナップショット。
type State struct {
	Downloading string  // ダウンロード中の版。無ければ空
	Staged      *Staged // 照合済みで入れ替え待ちの更新
	Failure     *Failure
}

// ManagerOptions は Manager の構成。
type ManagerOptions struct {
	Fetcher Fetcher
	Launch  Launcher
	// Busy は送信・下書きの保存の途中か。true のあいだは入れ替えを始めない。nil は常に安全とみなす。
	Busy func() bool
	// OnChange は状態が変わるたびに呼ばれる（画面への通知用。nil 可）。Manager のロックを外して呼ぶ。
	OnChange func(State)
	// Current はいま実行している版。
	Current string
	// Dir はダウンロードの保存先（Stager と同じ）。起動時に、途中のファイルと古い版のインストーラーを消す。
	Dir    string
	Logger *slog.Logger
}

// Manager は更新ファイルの用意（ダウンロード・照合）と入れ替えの指示を受け持つ。
// 入れ替えは、利用者が「再起動して更新」を選んだとき、または次にアプリを終了するときに、
// 送信中・下書きの保存中でなければ ApplyOnExit がインストーラーを起動して行う。
type Manager struct {
	fetcher  Fetcher
	launch   Launcher
	busy     func() bool
	onChange func(State)
	current  string
	dir      string
	log      *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	prepareMu sync.Mutex // Prepare を直列化する（ダウンロードの重複を避ける）

	mu          sync.Mutex
	downloading string
	staged      *Staged
	failure     *Failure
	gen         int  // Discard のたびに増える（Discard をまたいだダウンロードの結果を捨てるため）
	restart     bool // 「再起動して更新」が選ばれた（終了時にインストール後の起動し直しを付ける）
	noAuto      bool // 前回の入れ替えが完了しなかった。終了時の自動入れ替えはせず、利用者の操作だけで試す
}

// NewManager は Manager を作る。Dir があれば、途中のダウンロードと、入れ替え済み・不要なインストーラーを消し、
// 前回の入れ替えが完了していなかったか（記録した版が今の版より新しいまま）を調べる。
func NewManager(o ManagerOptions) *Manager {
	m := &Manager{
		fetcher: o.Fetcher, launch: o.Launch, busy: o.Busy, onChange: o.OnChange,
		current: o.Current, dir: o.Dir, log: o.Logger,
	}
	if m.log == nil {
		m.log = slog.Default()
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	if m.dir != "" {
		cleanStaging(m.dir, m.current)
		m.checkLastAttempt()
	}
	return m
}

// Close は実行中のダウンロードを中止して終わりを待つ。
func (m *Manager) Close() {
	m.cancel()
	m.wg.Wait()
}

func (m *Manager) checkLastAttempt() {
	path := filepath.Join(m.dir, attemptFile)
	raw, err := os.ReadFile(path) //nolint:gosec // 保存先フォルダ内の固定名
	if err != nil {
		return
	}
	v := strings.TrimSpace(string(raw))
	if !IsNewer(m.current, v) {
		_ = os.Remove(path) // 入れ替えが済んだ
		return
	}
	m.log.Warn("前回の更新の入れ替えが完了していません", "version", v, "current", m.current)
	m.failure = &Failure{Version: v, Message: interruptedMessage, interrupted: true}
	m.noAuto = true
}

// State は現在の状態を返す。
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stateLocked()
}

func (m *Manager) stateLocked() State {
	st := State{Downloading: m.downloading}
	if m.staged != nil {
		s := *m.staged
		st.Staged = &s
	}
	if m.failure != nil {
		f := *m.failure
		st.Failure = &f
	}
	return st
}

func (m *Manager) changed(st State) {
	if m.onChange != nil {
		m.onChange(st)
	}
}

// Prepare は rel の更新ファイルをダウンロードして照合し、入れ替え待ちにする。同じ版が用意済みなら何もしない。
// dev ビルド・今の版より新しくない版は ErrNotApplicable。失敗しても今の版のまま使い続けられる。
func (m *Manager) Prepare(ctx context.Context, rel Release) (Staged, error) {
	if IsDev(m.current) || !IsNewer(m.current, rel.Version) {
		return Staged{}, ErrNotApplicable
	}
	m.prepareMu.Lock()
	defer m.prepareMu.Unlock()

	m.mu.Lock()
	if m.staged != nil && m.staged.Version == rel.Version {
		s := *m.staged
		m.mu.Unlock()
		return s, nil
	}
	m.downloading = rel.Version
	gen := m.gen
	st := m.stateLocked()
	m.mu.Unlock()
	m.changed(st)

	staged, err := m.fetcher.Stage(ctx, rel)

	m.mu.Lock()
	m.downloading = ""
	if m.gen != gen { // ダウンロード中に Discard された（自動アップデートがオフになった）。結果は使わない
		if err == nil {
			_ = os.Remove(staged.Path)
		}
		st = m.stateLocked()
		m.mu.Unlock()
		m.changed(st)
		return Staged{}, ErrNotApplicable
	}
	if err != nil {
		m.failure = &Failure{Version: rel.Version, Message: UserMessage(err)}
		st = m.stateLocked()
		m.mu.Unlock()
		m.changed(st)
		return Staged{}, err
	}
	if m.failure != nil && !m.failure.interrupted {
		m.failure = nil // 通信・ハッシュ不一致の失敗は、成功したら版にかかわらず消す
	}
	if m.staged != nil && m.staged.Path != staged.Path {
		_ = os.Remove(m.staged.Path) // 古い版の用意済みファイル
	}
	m.staged = &staged
	st = m.stateLocked()
	m.mu.Unlock()
	m.changed(st)
	return staged, nil
}

// PrepareAsync は Prepare をバックグラウンドで行う（手動確認の応答を待たせない）。失敗はログに残し、状態に反映する。
func (m *Manager) PrepareAsync(rel Release) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if _, err := m.Prepare(m.ctx, rel); err != nil && !errors.Is(err, ErrNotApplicable) && m.ctx.Err() == nil {
			m.log.Warn("更新ファイルの用意に失敗", "version", rel.Version, "err", err)
		}
	}()
}

// Discard は用意済みの更新・失敗の表示・「再起動して更新」の指定を捨てる（自動アップデートをオフにしたとき）。
// ダウンロード中のものも、完了後に捨てる。
func (m *Manager) Discard() {
	m.mu.Lock()
	m.gen++
	m.discardLocked()
	m.failure = nil
	m.restart = false
	st := m.stateLocked()
	m.mu.Unlock()
	m.changed(st)
}

// discardLocked は用意済みの更新を空にしてファイルを消す。
func (m *Manager) discardLocked() {
	if m.staged != nil {
		_ = os.Remove(m.staged.Path)
		m.staged = nil
	}
}

func (m *Manager) isBusy() bool { return m.busy != nil && m.busy() }

// RequestRestart は「再起動して更新」。入れ替え待ちの更新があり、いま安全（送信中・下書きの保存中でない）なら、
// 終了時にインストール後に起動し直す指定を立てる。呼び出し側はこのあとアプリを終了する。
func (m *Manager) RequestRestart() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.staged == nil {
		return ErrNoUpdate
	}
	if m.isBusy() {
		return ErrBusy
	}
	m.restart = true
	return nil
}

// ApplyOnExit はアプリの終了時に呼ぶ。入れ替え待ちの更新があり、利用者が「再起動して更新」を選んでいるか、
// auto（自動アップデートが有効）で前回の入れ替えが完了しなかった版でなければ、いま安全な場合にインストーラーを起動して true を返す。
// 起動の直前にファイルを照合し直し、一致しなければ破棄する。送信中・下書きの保存中は ErrBusy（入れ替えず、次の終了に回す）。
func (m *Manager) ApplyOnExit(auto bool) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.staged == nil {
		return false, nil
	}
	if !m.restart && (!auto || m.noAuto) {
		return false, nil
	}
	if m.isBusy() {
		return false, ErrBusy
	}
	got, err := hashFile(m.staged.Path)
	if err != nil {
		m.discardLocked()
		return false, fmt.Errorf("update: 用意した更新ファイルを読めません: %w", err)
	}
	if got != m.staged.SHA256 {
		m.discardLocked()
		return false, ErrChecksumMismatch
	}
	m.recordAttempt(m.staged.Version)
	if err := m.launch(m.staged.Path, LaunchOptions{Relaunch: m.restart}); err != nil {
		m.clearAttempt()
		return false, fmt.Errorf("%w: %w", errLaunch, err)
	}
	return true, nil
}

func (m *Manager) recordAttempt(version string) {
	if m.dir == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(m.dir, attemptFile), []byte(version), 0o600); err != nil {
		m.log.Warn("更新の入れ替えの記録を書けません", "err", err)
	}
}

func (m *Manager) clearAttempt() {
	if m.dir != "" {
		_ = os.Remove(filepath.Join(m.dir, attemptFile))
	}
}
