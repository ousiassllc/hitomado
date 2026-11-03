package siwc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

// Settings は app_settings への読み書き（store.Store が満たす）。登録情報（秘密ではない）の保存先。
type Settings interface {
	GetSetting(ctx context.Context, key string) (value string, ok bool, err error)
	SetSetting(ctx context.Context, key, value string) error
}

// registration は app_settings の ai.chatgpt。秘密を含まない（ホスト ID・発行されたクライアント ID・検証済みの本人情報）。
// ログアウトしても残す（同じアカウントへ再ログインするときに同じクライアントを使うため）。
type registration struct {
	HostID   string `json:"hostId"`
	ClientID string `json:"clientId,omitempty"` // 発行されたクライアント ID（dynamic_agent_client は保存しない）
	Subject  string `json:"subject,omitempty"`  // 検証済み ID トークンの sub
	Email    string `json:"email,omitempty"`
	// ConsentNeeded は前回のログインで ChatGPT プランの利用が許可されなかったこと。次のログインで同意画面を求め直す。
	ConsentNeeded bool `json:"consentNeeded,omitempty"`
	// ExpiresAt はアクセストークンの期限（Unix ミリ秒）。
	ExpiresAt int64 `json:"expiresAt,omitempty"`
}

func (m *Manager) loadRegistration(ctx context.Context) (registration, error) {
	var r registration
	v, ok, err := m.opts.Settings.GetSetting(ctx, store.SettingAIChatGPT)
	if err != nil {
		return r, fmt.Errorf("%w: 設定を読み込めません", ErrFailed)
	}
	if ok {
		if err := json.Unmarshal([]byte(v), &r); err != nil {
			return registration{}, fmt.Errorf("%w: 設定を解釈できません", ErrFailed)
		}
	}
	return r, nil
}

func (m *Manager) saveRegistration(ctx context.Context, r registration) error {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("%w: 設定を保存できません", ErrFailed)
	}
	if err := m.opts.Settings.SetSetting(ctx, store.SettingAIChatGPT, string(b)); err != nil {
		return fmt.Errorf("%w: 設定を保存できません", ErrFailed)
	}
	return nil
}

// ensureHostID は登録にホスト ID があることを保証する（初回ログインの前に決めて保存する）。
// 形式は公式ドキュメントが認める `urn:uuid:<UUIDv4>`。
func (m *Manager) ensureHostID(ctx context.Context, r registration) (registration, error) {
	if r.HostID != "" {
		return r, nil
	}
	id, err := newHostID()
	if err != nil {
		return r, fmt.Errorf("%w: ホスト ID を作れません", ErrFailed)
	}
	r.HostID = id
	if err := m.saveRegistration(ctx, r); err != nil {
		return r, err
	}
	return r, nil
}

func newHostID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // variant 10
	h := fmt.Sprintf("%x", b)
	return "urn:uuid:" + h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

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

// 1 件が資格情報ストアの上限（Windows は約 2.5KB）を超えないよう、長い値は chunkSize 文字ずつに分けて保存する。
// 先頭のキーには値そのもの（短いとき）か、分割数を示す印を置く。トークンは ASCII（base64url）なのでバイト境界で切ってよい。
const (
	chunkSize   = 2000
	chunkMarker = "chunks:"
)

func chunkKey(base string, i int) string { return base + "#" + strconv.Itoa(i) }

// putSecret は値を保存する。空は削除。
// 先に新しい値を書き、そのあとで不要になった古い断片を消す（書き込みが失敗しても、更新前の値を先に失わない。
// 回転するリフレッシュトークンを失うと再ログインになるため）。
func (m *Manager) putSecret(kind, value string) error {
	base := secrets.ChatGPTTokenKey(kind)
	if value == "" {
		return m.dropSecret(kind)
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

// getSecret は値を返す。無ければ ""。
func (m *Manager) getSecret(kind string) (string, error) {
	base := secrets.ChatGPTTokenKey(kind)
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

// dropSecret は値（と分割保存の断片）を削除する。無くてもエラーにしない。
func (m *Manager) dropSecret(kind string) error {
	base := secrets.ChatGPTTokenKey(kind)
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

// loadTokens は保存済みのトークンを読む。無いものは ""。エラー文面に値は含まれない（資格情報ストアは値を返さない）。
func (m *Manager) loadTokens() (tokens, error) {
	var t tokens
	var err error
	if t.Refresh, err = m.getSecret(kindRefresh); err != nil {
		return tokens{}, fmt.Errorf("%w: 資格情報ストアから読み込めません", ErrFailed)
	}
	if t.Access, err = m.getSecret(kindAccess); err != nil {
		return tokens{}, fmt.Errorf("%w: 資格情報ストアから読み込めません", ErrFailed)
	}
	if t.ID, err = m.getSecret(kindID); err != nil {
		return tokens{}, fmt.Errorf("%w: 資格情報ストアから読み込めません", ErrFailed)
	}
	return t, nil
}

// saveTokens はトークンの組を保存する。リフレッシュトークン（更新のたびに入れ替わる）を最初に書く。
// ID トークンの空は「変更しない」ではなく削除なので、更新時に返らなかった ID トークンは呼び出し側が引き継いで渡す。
// ID トークンは任意（id_token_hint 用）なので、保存できなくても失敗にしない（断片を残さず捨てる）。
func (m *Manager) saveTokens(t tokens) error {
	if err := m.putSecret(kindRefresh, t.Refresh); err != nil {
		return fmt.Errorf("%w: 資格情報ストアへ保存できません", ErrFailed)
	}
	if err := m.putSecret(kindAccess, t.Access); err != nil {
		return fmt.Errorf("%w: 資格情報ストアへ保存できません", ErrFailed)
	}
	if err := m.putSecret(kindID, t.ID); err != nil {
		_ = m.dropSecret(kindID)
	}
	return nil
}

// clearTokens は保存済みのトークンをすべて消す（登録情報は残す）。
func (m *Manager) clearTokens() error {
	var errs []error
	for _, k := range []string{kindRefresh, kindAccess, kindID} {
		if err := m.dropSecret(k); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: 資格情報ストアから削除できません", ErrFailed)
	}
	return nil
}

// randomToken は暗号論的乱数の base64url（パディングなし）。state・nonce・PKCE verifier に使う。
func randomToken(nbytes int) (string, error) {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
