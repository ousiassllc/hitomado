package siwc

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ousiassllc/hitomado/internal/secrets"
)

// 資格情報ストアのトークンの種類（secrets.ChatGPTTokenKey の kind）。
const (
	kindAccess  = "access"
	kindRefresh = "refresh"
	kindID      = "id"
)

// tokens は資格情報ストアにあるトークンの組。
type tokens struct {
	Access, Refresh, ID string
}

// tokenBase は 1 つのアカウントのトークンを置くキー（種類ごと）。
type tokenBase struct{ access, refresh, id string }

// tokenBases はアカウント（client_id）のトークンのキー。別のアカウントのキーと重ならない。
func tokenBases(clientID string) tokenBase {
	return tokenBase{
		access:  secrets.ChatGPTTokenKey(clientID, kindAccess),
		refresh: secrets.ChatGPTTokenKey(clientID, kindRefresh),
		id:      secrets.ChatGPTTokenKey(clientID, kindID),
	}
}

func legacyTokenBases() tokenBase {
	return tokenBase{
		access:  secrets.LegacyChatGPTTokenKey(kindAccess),
		refresh: secrets.LegacyChatGPTTokenKey(kindRefresh),
		id:      secrets.LegacyChatGPTTokenKey(kindID),
	}
}

// 1 件が資格情報ストアの上限（Windows は約 2.5KB）を超えないよう、長い値は chunkSize 文字ずつに分けて保存する。
// 先頭のキーには値そのもの（短いとき）か、分割数を示す印を置く。トークンは ASCII（base64url）なのでバイト境界で切ってよい。
const (
	chunkSize   = 2000
	chunkMarker = "chunks:"
)

func chunkKey(base string, i int) string { return base + "#" + strconv.Itoa(i) }

// putSecret は base に値を保存する。空は削除。
// 先に新しい値を書き、そのあとで不要になった古い断片を消す（書き込みが失敗しても、更新前の値を先に失わない。
// 回転するリフレッシュトークンを失うと再ログインになるため）。
func (m *Manager) putSecret(base, value string) error {
	if value == "" {
		return m.dropSecret(base)
	}
	oldN := m.chunkCount(base)
	newN := 0
	if len(value) <= chunkSize {
		if err := m.opts.Secrets.Set(base, value); err != nil {
			return err
		}
	} else {
		newN = (len(value) + chunkSize - 1) / chunkSize
		for i := range newN {
			end := min((i+1)*chunkSize, len(value))
			if err := m.opts.Secrets.Set(chunkKey(base, i), value[i*chunkSize:end]); err != nil {
				return err
			}
		}
		if err := m.opts.Secrets.Set(base, chunkMarker+strconv.Itoa(newN)); err != nil {
			return err
		}
	}
	for i := newN; i < oldN; i++ { // 短くなった分の古い断片
		if err := m.opts.Secrets.Delete(chunkKey(base, i)); err != nil {
			return err
		}
	}
	return nil
}

// chunkCount は base が分割保存されているときの断片数（そうでなければ 0）。
func (m *Manager) chunkCount(base string) int {
	head, err := m.opts.Secrets.Get(base)
	if err != nil {
		return 0
	}
	rest, ok := strings.CutPrefix(head, chunkMarker)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 2 || n > 64 {
		return 0
	}
	return n
}

// getSecret は base の値を返す。無ければ ""。
func (m *Manager) getSecret(base string) (string, error) {
	head, err := m.opts.Secrets.Get(base)
	if errors.Is(err, secrets.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	rest, ok := strings.CutPrefix(head, chunkMarker)
	if !ok {
		return head, nil
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 2 || n > 64 {
		return "", nil // 形式が不正な値は、無いものとして扱う（状態の取得を壊さない。再ログインで置き換わる）
	}
	var b strings.Builder
	for i := range n {
		part, err := m.opts.Secrets.Get(chunkKey(base, i))
		if err != nil {
			if errors.Is(err, secrets.ErrNotFound) {
				return "", nil // 断片が欠けた値は使えない。無いものとして扱う
			}
			return "", err
		}
		b.WriteString(part)
	}
	return b.String(), nil
}

// dropSecret は base の値（と分割保存の断片）を削除する。無くてもエラーにしない。
func (m *Manager) dropSecret(base string) error {
	head, err := m.opts.Secrets.Get(base)
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return err
	}
	if rest, ok := strings.CutPrefix(head, chunkMarker); ok {
		if n, aerr := strconv.Atoi(rest); aerr == nil && n >= 2 && n <= 64 {
			for i := range n {
				if err := m.opts.Secrets.Delete(chunkKey(base, i)); err != nil {
					return err
				}
			}
		}
	}
	return m.opts.Secrets.Delete(base)
}

// loadTokens はアカウントの保存済みのトークンを読む。無いものは ""。エラー文面に値は含まれない（資格情報ストアは値を返さない）。
func (m *Manager) loadTokens(clientID string) (tokens, error) {
	return m.loadTokensAt(tokenBases(clientID))
}

func (m *Manager) loadTokensAt(b tokenBase) (tokens, error) {
	var t tokens
	var err error
	if t.Refresh, err = m.getSecret(b.refresh); err != nil {
		return tokens{}, fmt.Errorf("%w: 資格情報ストアから読み込めません", ErrFailed)
	}
	if t.Access, err = m.getSecret(b.access); err != nil {
		return tokens{}, fmt.Errorf("%w: 資格情報ストアから読み込めません", ErrFailed)
	}
	if t.ID, err = m.getSecret(b.id); err != nil {
		return tokens{}, fmt.Errorf("%w: 資格情報ストアから読み込めません", ErrFailed)
	}
	return t, nil
}

// saveTokens はアカウントのトークンの組を保存する。リフレッシュトークン（更新のたびに入れ替わる）を最初に書く。
// ID トークンの空は「変更しない」ではなく削除なので、更新時に返らなかった ID トークンは呼び出し側が引き継いで渡す。
// ID トークンは任意（id_token_hint 用）なので、保存できなくても失敗にしない（断片を残さず捨てる）。
func (m *Manager) saveTokens(clientID string, t tokens) error {
	return m.saveTokensAt(tokenBases(clientID), t)
}

func (m *Manager) saveTokensAt(b tokenBase, t tokens) error {
	if err := m.putSecret(b.refresh, t.Refresh); err != nil {
		return fmt.Errorf("%w: 資格情報ストアへ保存できません", ErrFailed)
	}
	if err := m.putSecret(b.access, t.Access); err != nil {
		return fmt.Errorf("%w: 資格情報ストアへ保存できません", ErrFailed)
	}
	if err := m.putSecret(b.id, t.ID); err != nil {
		_ = m.dropSecret(b.id)
	}
	return nil
}

// clearTokens はアカウントの保存済みのトークンをすべて消す（登録情報は残す。ほかのアカウントには触れない）。
func (m *Manager) clearTokens(clientID string) error {
	return m.clearTokensAt(tokenBases(clientID))
}

func (m *Manager) clearTokensAt(b tokenBase) error {
	var errs []error
	for _, k := range []string{b.refresh, b.access, b.id} {
		if err := m.dropSecret(k); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: 資格情報ストアから削除できません", ErrFailed)
	}
	return nil
}

// hasSession はアカウントにログイン中のセッションがあるか（リフレッシュトークンの先頭の要素が資格情報ストアにある）。値の全体は読まない。
func (m *Manager) hasSession(clientID string) (bool, error) {
	head, err := m.opts.Secrets.Get(tokenBases(clientID).refresh)
	if errors.Is(err, secrets.ErrNotFound) || (err == nil && head == "") {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: 資格情報ストアから読み込めません", ErrFailed)
	}
	return true, nil
}

// randomToken は暗号論的乱数の base64url（パディングなし）。state・nonce・PKCE verifier に使う。
func randomToken(nbytes int) (string, error) {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
