package siwc

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/ousiassllc/hitomado/internal/store"
)

// Settings は app_settings への読み書き（store.Store が満たす）。登録情報（秘密ではない）の保存先。
type Settings interface {
	GetSetting(ctx context.Context, key string) (value string, ok bool, err error)
	SetSetting(ctx context.Context, key, value string) error
}

// account は ChatGPT アカウント 1 件の登録（クライアント登録）。秘密を含まない。
// 識別子は発行された client_id。同じメールアドレスでも、別の client_id なら別の登録として扱う。
// ログアウトしても残す（同じアカウントへ再ログインするときに同じクライアントを使うため）。
type account struct {
	ClientID string `json:"clientId"`          // 発行されたクライアント ID（dynamic_agent_client は保存しない）
	Subject  string `json:"subject,omitempty"` // 検証済み ID トークンの sub
	Email    string `json:"email,omitempty"`
	// ConsentNeeded は前回のログインで ChatGPT プランの利用が許可されなかったこと。次のログインで同意画面を求め直す。
	ConsentNeeded bool `json:"consentNeeded,omitempty"`
	// ExpiresAt はアクセストークンの期限（Unix ミリ秒）。
	ExpiresAt int64 `json:"expiresAt,omitempty"`
}

// registry は app_settings の ai.chatgpt。秘密を含まない（ホスト ID・使用中のアカウント・アカウントごとの登録）。
type registry struct {
	// HostID はこの端末の ext_agent_host_id（全アカウントで共通）。
	HostID string `json:"hostId"`
	// Active は使用中のアカウントの client_id。
	Active   string    `json:"active,omitempty"`
	Accounts []account `json:"accounts,omitempty"`
}

func (r *registry) find(clientID string) *account {
	if clientID == "" {
		return nil
	}
	for i := range r.Accounts {
		if r.Accounts[i].ClientID == clientID {
			return &r.Accounts[i]
		}
	}
	return nil
}

// upsert は client_id の登録を返す。無ければ末尾に作る（登録順を保つ）。
func (r *registry) upsert(clientID string) *account {
	if a := r.find(clientID); a != nil {
		return a
	}
	r.Accounts = append(r.Accounts, account{ClientID: clientID})
	return &r.Accounts[len(r.Accounts)-1]
}

// legacyRegistration は v0.1.7 までの ai.chatgpt（アカウント 1 件を直接持つ形）。移行のときだけ読む。
type legacyRegistration struct {
	HostID        string `json:"hostId"`
	ClientID      string `json:"clientId"`
	Subject       string `json:"subject"`
	Email         string `json:"email"`
	ConsentNeeded bool   `json:"consentNeeded"`
	ExpiresAt     int64  `json:"expiresAt"`
}

// loadRegistry は登録を読む。旧形式（アカウント 1 件）は、最初に呼ばれたときに複数アカウントの形へ移行する。
func (m *Manager) loadRegistry(ctx context.Context) (registry, error) {
	if err := m.migrate(ctx); err != nil {
		return registry{}, err
	}
	return m.readRegistry(ctx)
}

func (m *Manager) readRegistry(ctx context.Context) (registry, error) {
	var r registry
	v, ok, err := m.opts.Settings.GetSetting(ctx, store.SettingAIChatGPT)
	if err != nil {
		return r, fmt.Errorf("%w: 設定を読み込めません", ErrFailed)
	}
	if ok {
		if err := json.Unmarshal([]byte(v), &r); err != nil {
			return registry{}, fmt.Errorf("%w: 設定を解釈できません", ErrFailed)
		}
	}
	return r, nil
}

func (m *Manager) writeRegistry(ctx context.Context, r registry) error {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("%w: 設定を保存できません", ErrFailed)
	}
	if err := m.opts.Settings.SetSetting(ctx, store.SettingAIChatGPT, string(b)); err != nil {
		return fmt.Errorf("%w: 設定を保存できません", ErrFailed)
	}
	return nil
}

// updateRegistry は登録を読み直して fn で書き換え、保存する（読み書きを regMu で直列化する）。
// 通信をまたいで古い登録を書き戻さないよう、変更は必ずここを通し、fn には変えたい項目だけを書く。
func (m *Manager) updateRegistry(ctx context.Context, fn func(*registry) error) error {
	if err := m.migrate(ctx); err != nil {
		return err
	}
	m.regMu.Lock()
	defer m.regMu.Unlock()
	r, err := m.readRegistry(ctx)
	if err != nil {
		return err
	}
	if err := fn(&r); err != nil {
		return err
	}
	return m.writeRegistry(ctx, r)
}

// migrate は旧形式（ai.chatgpt が client_id を直接持ち、トークンが hitomado/chatgpt_token/<kind>）を
// 複数アカウントの形（アカウントごとの client_id のキー）へ移す。移行済みなら何もしない。
// 途中で失敗しても、再実行で同じ結果になる（新しい保存先へ書いてから登録を置き換え、旧キーを最後に消す）。
func (m *Manager) migrate(ctx context.Context) error {
	if m.migrated.Load() {
		return nil
	}
	m.migrateMu.Lock()
	defer m.migrateMu.Unlock()
	if m.migrated.Load() {
		return nil
	}
	v, ok, err := m.opts.Settings.GetSetting(ctx, store.SettingAIChatGPT)
	if err != nil {
		return fmt.Errorf("%w: 設定を読み込めません", ErrFailed)
	}
	var old legacyRegistration
	if ok {
		if err := json.Unmarshal([]byte(v), &old); err != nil {
			return fmt.Errorf("%w: 設定を解釈できません", ErrFailed)
		}
	}
	if old.ClientID == "" {
		m.migrated.Store(true)
		return nil
	}

	var t tokens
	if t, err = m.loadTokensAt(legacyTokenBases()); err != nil {
		return err
	}
	signedIn := t.Refresh != ""
	if signedIn {
		if err := m.saveTokensAt(tokenBases(old.ClientID), t); err != nil {
			return err
		}
	}
	reg := registry{
		HostID: old.HostID,
		Accounts: []account{{
			ClientID: old.ClientID, Subject: old.Subject, Email: old.Email,
			ConsentNeeded: old.ConsentNeeded, ExpiresAt: old.ExpiresAt,
		}},
	}
	if signedIn {
		reg.Active = old.ClientID
	}
	m.regMu.Lock()
	err = m.writeRegistry(ctx, reg)
	m.regMu.Unlock()
	if err != nil {
		return err
	}
	if err := m.clearTokensAt(legacyTokenBases()); err != nil {
		return err
	}
	m.migrated.Store(true)
	return nil
}

// ensureHostID は登録にホスト ID があることを保証して返す（初回ログインの前に決めて保存する）。
// 形式は公式ドキュメントが認める `urn:uuid:<UUIDv4>`。
func (m *Manager) ensureHostID(ctx context.Context) (string, error) {
	var id string
	err := m.updateRegistry(ctx, func(r *registry) error {
		if r.HostID == "" {
			h, err := newHostID()
			if err != nil {
				return fmt.Errorf("%w: ホスト ID を作れません", ErrFailed)
			}
			r.HostID = h
		}
		id = r.HostID
		return nil
	})
	return id, err
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
