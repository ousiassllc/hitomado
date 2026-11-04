package oauth

import (
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/secrets"
)

// trackerKey は「認可の途中」を表す印のキー（資格情報ストア。秘密は入れない）。
const trackerKey = "hitomado/oauth_in_progress"

// Tracker は認可の進行中を資格情報ストアへ印として残し、認可の途中でプロセスが止められたこと
// （Android がブラウザに出ている間にアプリのプロセスを終了させた場合など）を次の起動で検出する。
//
// ループバックのリダイレクト（127.0.0.1 の待受）はプロセスと一緒に消えるため、プロセスが止められた認可は
// 続きから完了できない。保存しても完了に使えない state と PKCE verifier は残さず（残すのは時刻だけ）、
// 利用者に「ログインはやり直し」と案内するための検出だけを行う（docs/environment/android.md §14）。
//
// 前回の印は最初に必要になった時点で 1 回だけ読んで消す（起動直後の JNI 呼び出しを避けるため、構築時には読まない）。
// nil の *Tracker は何もしない（Windows）。
type Tracker struct {
	kv secrets.Store

	mu          sync.Mutex
	loaded      bool
	interrupted bool
}

// NewTracker は kv を使う Tracker を返す。kv の読み書きの失敗は検出を諦めるだけで、認可そのものには影響しない。
func NewTracker(kv secrets.Store) *Tracker { return &Tracker{kv: kv} }

// loadLocked は前回の起動が残した印を 1 回だけ取り込み、消す。mu を持って呼ぶ。
func (t *Tracker) loadLocked() {
	if t.loaded {
		return
	}
	t.loaded = true
	if _, err := t.kv.Get(trackerKey); err == nil {
		t.interrupted = true
		_ = t.kv.Delete(trackerKey)
	}
}

// Begin は認可の開始（ブラウザを開く前）に呼ぶ。前回の印は先に取り込んでから、今回の印を残す。
func (t *Tracker) Begin() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loadLocked()
	_ = t.kv.Set(trackerKey, time.Now().UTC().Format(time.RFC3339))
}

// End は認可の終了（成功・失敗・キャンセル・タイムアウトのいずれでも）に呼ぶ。
func (t *Tracker) End() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loadLocked()
	_ = t.kv.Delete(trackerKey)
}

// TakeInterrupted は、前回の起動で認可が途中で止められていたかを 1 回だけ返す（2 回目以降は false）。
func (t *Tracker) TakeInterrupted() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loadLocked()
	v := t.interrupted
	t.interrupted = false
	return v
}
