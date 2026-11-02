// Package confirm は送信確認トークンの発行・検証を提供する（F-06／API §5.2）。
// トークンはセッションに紐づけてメモリのみで保持し、TTL 付き・1 回限りで失効する。
package confirm

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// Clock は現在時刻の供給元（internal/clock.Clock と互換）。
type Clock interface{ Now() time.Time }

// Token は確認トークン（128bit 乱数の hex）。
type Token string

// Verify の失敗結果。
var (
	// ErrRequired はトークンが無い・未知（無効化済みを含む）ことを表す（CONFIRM_REQUIRED）。
	ErrRequired = errors.New("送信前の確認が必要です")
	// ErrExpired はトークンの期限切れ・使用済みを表す（CONFIRM_EXPIRED）。
	ErrExpired = errors.New("送信確認の有効期限が切れています。もう一度確認してください")
	// ErrMismatch は確認後に内容が変更されたことを表す（CONFIRM_MISMATCH）。
	ErrMismatch = errors.New("確認後に内容が変更されました。もう一度確認してください")
)

// pruneGrace は期限切れエントリを保持し続ける時間（メモリ滞留の上限）。
const pruneGrace = time.Hour

type entry struct {
	token   Token
	hash    Hash
	expires time.Time
	used    bool
}

// Manager は確認トークンを管理する。並行利用できる。
type Manager struct {
	clock Clock
	ttl   time.Duration

	mu      sync.Mutex
	entries map[string]*entry // セッション ID → 現在のトークン（セッションごとに 1 つ）
}

// NewManager は TTL が ttl の Manager を返す。
func NewManager(c Clock, ttl time.Duration) *Manager {
	return &Manager{clock: c, ttl: ttl, entries: map[string]*entry{}}
}

// Issue は h に紐づく新しいトークンと失効時刻を返す。同一セッションの旧トークンは無効になる。
func (m *Manager) Issue(sessionID string, h Hash) (Token, time.Time) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("confirm: 乱数を取得できません: " + err.Error())
	}
	tok := Token(hex.EncodeToString(b[:]))

	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	m.pruneLocked(now)
	exp := now.Add(m.ttl)
	m.entries[sessionID] = &entry{token: tok, hash: h, expires: exp}
	return tok, exp
}

// Verify はトークンを検証し、成功したらそのトークンを使用済みにする（1 回限り）。
// 空・未知は ErrRequired、使用済み・期限切れは ErrExpired、内容ハッシュ不一致は ErrMismatch
// （不一致ではトークンを消費しない。期限内なら内容を戻して再送できる）。
func (m *Manager) Verify(sessionID string, t Token, h Hash) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[sessionID]
	if t == "" || !ok || subtle.ConstantTimeCompare([]byte(e.token), []byte(t)) != 1 {
		return ErrRequired
	}
	if e.used || !m.clock.Now().Before(e.expires) {
		return ErrExpired
	}
	if subtle.ConstantTimeCompare(e.hash[:], h[:]) != 1 {
		return ErrMismatch
	}
	e.used = true
	return nil
}

// Invalidate はセッションのトークンを破棄する（セッション終了・内容変更時）。
func (m *Manager) Invalidate(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, sessionID)
}

// pruneLocked は期限から pruneGrace を過ぎたエントリを捨てる。期限直後は残しておき、
// 期限切れ（ErrExpired）と未知（ErrRequired）を区別できるようにする。
func (m *Manager) pruneLocked(now time.Time) {
	for id, e := range m.entries {
		if now.After(e.expires.Add(pruneGrace)) {
			delete(m.entries, id)
		}
	}
}
