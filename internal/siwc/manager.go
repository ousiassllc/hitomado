package siwc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

const (
	// DefaultIssuer は OpenAI の認可サーバー。エンドポイントと JWKS は /.well-known/openid-configuration から得る。
	DefaultIssuer = "https://auth.openai.com"

	// AgentName は動的登録で名乗る名前（agent_name_hint）。Hitomado 自身の名前を、全インストールで同じにする。
	// OpenAI やほかのアプリを名乗らない。
	AgentName = "Hitomado"

	// DefaultTimeout は認可待ち（ブラウザでの操作）の上限。
	DefaultTimeout = 5 * time.Minute

	httpTimeout = 30 * time.Second
	// refreshSkew はアクセストークンの期限のこの手前から更新する（docs: 期限の近くで更新）。
	refreshSkew = 60 * time.Second
	// revokeAttempts は失効の試行回数（通信失敗・5xx は短い間隔で再試行する。docs: Sign out）。
	revokeAttempts = 3
)

// Options は Manager の構成。OpenBrowser・Secrets・Settings は必須。
type Options struct {
	// OpenBrowser は認可 URL をシステムブラウザで開く。URL は id_token_hint を含み得るのでログに出さないこと。
	OpenBrowser func(url string) error
	Secrets     secrets.Store
	Settings    Settings
	Clock       clock.Clock // nil なら実時間
	Logger      *slog.Logger
	// Issuer は認可サーバー（既定 DefaultIssuer。テスト用に差し替える）。
	Issuer string
	// HTTPClient は OpenAI への通信（nil なら既定。リダイレクトは追従しない）。
	HTTPClient *http.Client
	// Timeout は認可待ちの上限（既定 DefaultTimeout）。
	Timeout time.Duration
	// RevokeBackoff は失効の再試行の間隔（既定 500ms。テスト用）。
	RevokeBackoff time.Duration
}

// Manager は ChatGPT ログインの状態（登録・トークン）を管理する。トークンの更新は直列化する。
type Manager struct {
	opts Options
	hc   *http.Client
	clk  clock.Clock

	// refreshMu はトークンの読み書き（更新・ログイン保存・ログアウト）を直列化する。回転するリフレッシュトークンを競合させない。
	refreshMu sync.Mutex

	// regMu は登録（app_settings の ai.chatgpt）の読み書きを直列化する。通信中は持たない（短い区間だけ）。
	regMu sync.Mutex
	// migrateMu・migrated は旧形式（アカウント 1 件）の登録・トークンの移行（registry.go の migrate）。
	migrateMu sync.Mutex
	migrated  atomic.Bool

	mu         sync.Mutex // cancelSign（通信中に持たない。CancelSignIn がすぐ効くように）
	cancelSign context.CancelCauseFunc

	discMu    sync.Mutex // provider・revokeURL（discovery の通信中は持つが、mu とは別）
	provider  *oidc.Provider
	revokeURL string
}

// New は Manager を作る。
func New(opts Options) *Manager {
	if opts.Issuer == "" {
		opts.Issuer = DefaultIssuer
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	if opts.RevokeBackoff <= 0 {
		opts.RevokeBackoff = 500 * time.Millisecond
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: httpTimeout}
	}
	// リダイレクトは追従しない（トークン・失効の POST を別のホストへ再送しない）。呼び出し元の Client は複製して設定する。
	c := *hc
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var clk clock.Clock = clock.System{}
	if opts.Clock != nil {
		clk = opts.Clock
	}
	return &Manager{opts: opts, hc: &c, clk: clk}
}

// discover は OpenID Connect のメタデータ（エンドポイント・JWKS の場所）を取得して保持する。
func (m *Manager) discover(ctx context.Context) (*oidc.Provider, error) {
	m.discMu.Lock()
	defer m.discMu.Unlock()
	if m.provider != nil {
		return m.provider, nil
	}
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, m.hc), m.opts.Issuer)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("siwc: 要求を中断しました: %w", ctx.Err())
		}
		var ue *url.Error
		if errors.As(err, &ue) {
			return nil, fmt.Errorf("siwc: 通信に失敗しました: %w", ue)
		}
		return nil, fmt.Errorf("%w: 認可サーバーの情報を取得できません", ErrFailed)
	}
	var meta struct {
		Revocation string `json:"revocation_endpoint"`
	}
	if err := p.Claims(&meta); err != nil || meta.Revocation == "" {
		return nil, fmt.Errorf("%w: 認可サーバーの情報が不正です", ErrInvalidResponse)
	}
	m.provider, m.revokeURL = p, meta.Revocation
	return p, nil
}

func (m *Manager) tokenEndpoint(ctx context.Context) (string, error) {
	p, err := m.discover(ctx)
	if err != nil {
		return "", err
	}
	return p.Endpoint().TokenURL, nil
}

func (m *Manager) revocationEndpoint(ctx context.Context) (string, error) {
	if _, err := m.discover(ctx); err != nil {
		return "", err
	}
	return m.revokeURL, nil
}

// Status は Auth の実装。通信せず、更新・ログアウトの途中でも待たない（refreshMu を取らない）。
// ログイン済みかは「登録があり、リフレッシュトークンの先頭の要素が資格情報ストアにある」ことで判断する（値の全体は読まない）。
func (m *Manager) Status(ctx context.Context) (Status, error) {
	reg, err := m.loadRegistry(ctx)
	if err != nil {
		return Status{}, err
	}
	st := Status{Accounts: make([]Account, 0, len(reg.Accounts))}
	for _, a := range reg.Accounts {
		signedIn, err := m.hasSession(a.ClientID)
		if err != nil {
			return Status{}, err
		}
		active := a.ClientID == reg.Active
		st.Accounts = append(st.Accounts, Account{ID: a.ClientID, Email: a.Email, SignedIn: signedIn, Active: active})
		if active {
			st.SignedIn, st.Email = signedIn, a.Email
		}
	}
	return st, nil
}

// AccessToken は Auth の実装。
func (m *Manager) AccessToken(ctx context.Context) (string, error) { return m.access(ctx, false) }

// ForceRefresh は Auth の実装。
func (m *Manager) ForceRefresh(ctx context.Context) (string, error) { return m.access(ctx, true) }

// access は使用中のアカウントのアクセストークンを返す。
func (m *Manager) access(ctx context.Context, force bool) (string, error) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	reg, err := m.loadRegistry(ctx)
	if err != nil {
		return "", err
	}
	acct := reg.find(reg.Active)
	if acct == nil {
		return "", ErrNotSignedIn
	}
	t, err := m.loadTokens(acct.ClientID)
	if err != nil {
		return "", err
	}
	if t.Refresh == "" {
		return "", ErrNotSignedIn
	}
	valid := t.Access != "" && acct.ExpiresAt > 0 &&
		m.clk.Now().Add(refreshSkew).Before(time.UnixMilli(acct.ExpiresAt))
	if valid && !force {
		return t.Access, nil
	}
	return m.refreshLocked(ctx, acct.ClientID, t)
}

// refreshLocked はアカウントのリフレッシュトークンで更新し、置き換えを保存する（refreshMu を持って呼ぶ）。
// 更新できない（再ログインが必要な）ときはそのアカウントのトークンを消して ErrReauthRequired。通信失敗・5xx は保存済みを残して返す。
func (m *Manager) refreshLocked(ctx context.Context, clientID string, t tokens) (string, error) {
	tr, err := m.requestTokens(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {t.Refresh},
		"resource":      {resource},
	})
	if err != nil {
		var se *serverError
		if errors.As(err, &se) {
			switch {
			case unusableRefreshCodes[se.Code]:
				if cerr := m.clearTokens(clientID); cerr != nil {
					return "", cerr
				}
				return "", fmt.Errorf("%w: 更新できません（%s）", ErrReauthRequired, se.Code)
			case se.Code == "invalid_client":
				return "", fmt.Errorf("%w: クライアントの設定が不正です", ErrFailed)
			}
		}
		return "", err
	}
	if tr.RefreshToken == "" {
		// 回転しない場合は既存を使い続ける（docs は置き換えを返すとしているが、欠けても今のトークンは有効）。
		tr.RefreshToken = t.Refresh
	}
	if tr.IDToken == "" {
		tr.IDToken = t.ID
	}
	if err := m.saveTokens(clientID, tokens{Access: tr.AccessToken, Refresh: tr.RefreshToken, ID: tr.IDToken}); err != nil {
		return "", err
	}
	// 通信の間に切り替え・削除があり得るので、登録は読み直して、このアカウントの期限だけを書く。
	expires := m.expiry(tr.ExpiresIn)
	if err := m.updateRegistry(ctx, func(r *registry) error {
		if a := r.find(clientID); a != nil {
			a.ExpiresAt = expires
		}
		return nil
	}); err != nil {
		return "", err
	}
	return tr.AccessToken, nil
}

func (m *Manager) expiry(expiresIn int64) int64 {
	if expiresIn <= 0 {
		expiresIn = 3600 // docs: アクセストークンは 1 時間
	}
	return m.clk.Now().Add(time.Duration(expiresIn) * time.Second).UnixMilli()
}

// CancelSignIn は Auth の実装。
func (m *Manager) CancelSignIn() {
	m.mu.Lock()
	cancel := m.cancelSign
	m.mu.Unlock()
	if cancel != nil {
		cancel(ErrCancelled)
	}
}
