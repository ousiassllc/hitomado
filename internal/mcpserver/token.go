package mcpserver

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/ousiassllc/hitomado/internal/secrets"
)

// TokenPrefix は Hitomado が発行するトークンの接頭辞（ログの伏せ処理が拾えるように固定する）。
const TokenPrefix = "hmcp_"

// Tokens は MCP の Bearer トークンを資格情報ストアで管理する。トークンは 1 つだけで、再発行で古いものは無効になる。
// ストアの値を毎回読まずに済むよう、ハッシュをメモリに持つ。
type Tokens struct {
	store secrets.Store

	mu     sync.Mutex
	loaded bool
	hash   [sha256.Size]byte
	has    bool
}

// NewTokens は Tokens を作る。
func NewTokens(store secrets.Store) *Tokens { return &Tokens{store: store} }

func (t *Tokens) load() error {
	if t.loaded {
		return nil
	}
	v, err := t.store.Get(secrets.MCPTokenKey())
	switch {
	case errors.Is(err, secrets.ErrNotFound):
		t.has = false
	case err != nil:
		return fmt.Errorf("read mcp token: %w", err)
	default:
		t.hash, t.has = sha256.Sum256([]byte(v)), v != ""
	}
	t.loaded = true
	return nil
}

// Issue は新しいトークンを作って保存し、返す（古いトークンは無効になる）。返した値は呼び出し側が利用者へ見せる 1 回だけで、
// 以後は取り出せない。
func (t *Tokens) Issue() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate mcp token: %w", err)
	}
	token := TokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.store.Set(secrets.MCPTokenKey(), token); err != nil {
		return "", fmt.Errorf("save mcp token: %w", err)
	}
	t.hash, t.has, t.loaded = sha256.Sum256([]byte(token)), true, true
	return token, nil
}

// Revoke はトークンを削除する（以後すべての接続を拒否する）。
func (t *Tokens) Revoke() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	// ストアの削除が失敗しても、このプロセスでは先に無効にする（再起動すると残ったトークンが復活するので、エラーも返す）。
	t.has, t.loaded = false, true
	if err := t.store.Delete(secrets.MCPTokenKey()); err != nil {
		return fmt.Errorf("delete mcp token: %w", err)
	}
	return nil
}

// Has はトークンが発行済みかを返す。
func (t *Tokens) Has() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.load() == nil && t.has
}

// Verify は candidate が発行済みのトークンと一致するかを定数時間で比べる。未発行・読み取り失敗は false。
func (t *Tokens) Verify(candidate string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.load() != nil || !t.has || candidate == "" {
		return false
	}
	got := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(got[:], t.hash[:]) == 1
}
