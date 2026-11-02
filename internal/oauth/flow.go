package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Google の本番エンドポイント（Options で上書きできる。テスト用）。
const (
	DefaultAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	DefaultTokenURL    = "https://oauth2.googleapis.com/token" //nolint:gosec // 資格情報ではなくエンドポイント URL
	DefaultUserinfoURL = "https://openidconnect.googleapis.com/v1/userinfo"
	DefaultRevokeURL   = "https://oauth2.googleapis.com/revoke"

	// DefaultTimeout は認可の待ち時間の上限（NF-S03）。
	DefaultTimeout = 5 * time.Minute

	gmailModifyScope = "https://www.googleapis.com/auth/gmail.modify"
	httpTimeout      = 30 * time.Second

	// DefaultExchangeTimeout はコード受領後のトークン交換・userinfo 取得の期限。
	DefaultExchangeTimeout = 30 * time.Second
)

// Options は Flow の構成。ゼロ値のフィールドは既定値になる（OpenBrowser のみ必須）。
type Options struct {
	// OpenBrowser は認可 URL をシステムブラウザで開く（Wails 側が実装して注入する）。
	OpenBrowser func(url string) error
	AuthURL     string
	TokenURL    string
	UserinfoURL string
	RevokeURL   string
	HTTPClient  *http.Client
	// Timeout は認可待ち（ブラウザでの承認まで）の上限。既定 5 分。
	Timeout time.Duration
	// ExchangeTimeout はコード受領後のトークン交換と userinfo 取得に与える期限。既定 30 秒。
	ExchangeTimeout time.Duration
}

type flow struct {
	opts Options
}

// NewFlow は Google 向けの認可フローを返す。
func NewFlow(opts Options) Flow {
	if opts.AuthURL == "" {
		opts.AuthURL = DefaultAuthURL
	}
	if opts.TokenURL == "" {
		opts.TokenURL = DefaultTokenURL
	}
	if opts.UserinfoURL == "" {
		opts.UserinfoURL = DefaultUserinfoURL
	}
	if opts.RevokeURL == "" {
		opts.RevokeURL = DefaultRevokeURL
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: httpTimeout}
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	if opts.ExchangeTimeout <= 0 {
		opts.ExchangeTimeout = DefaultExchangeTimeout
	}
	return &flow{opts: opts}
}

// Authorize は Flow の実装。待受は戻る時点で必ず閉じている。
// ログ・エラーにトークン・認可コード・state を含めない。
func (f *flow) Authorize(parent context.Context, cfg ClientConfig) (Result, error) {
	if f.opts.OpenBrowser == nil {
		return Result{}, errors.New("oauth: OpenBrowser が設定されていません")
	}
	ctx, cancel := context.WithTimeout(parent, f.opts.Timeout)
	defer cancel()

	state, err := randomString()
	if err != nil {
		return Result{}, err
	}
	verifier, err := randomString()
	if err != nil {
		return Result{}, err
	}
	srv, err := startCallbackServer(state)
	if err != nil {
		return Result{}, err
	}
	defer srv.close()

	authURL, err := f.authURL(cfg, srv.redirectURI(), state, verifier)
	if err != nil {
		return Result{}, err
	}
	if err := f.opts.OpenBrowser(authURL); err != nil {
		return Result{}, fmt.Errorf("oauth: ブラウザを開けませんでした: %w", err)
	}

	var cb callbackResult
	select {
	case cb = <-srv.result:
	case <-ctx.Done():
		return Result{}, ctxError(parent, ctx)
	}
	srv.close() // コールバック受信後は待受を閉じてからトークン交換へ進む
	if cb.err != nil {
		return Result{}, cb.err
	}

	// 認可待ちの期限（Timeout）とは別に、交換と userinfo には新しい期限（ExchangeTimeout）を与える。
	xctx, xcancel := context.WithTimeout(parent, f.opts.ExchangeTimeout)
	defer xcancel()
	res, err := f.complete(xctx, cfg, cb.code, verifier, srv.redirectURI())
	if err != nil {
		if xctx.Err() != nil {
			return Result{}, ctxError(parent, xctx)
		}
		return Result{}, err
	}
	return res, nil
}

// complete はコード交換→スコープ検証→userinfo 取得を行う。
func (f *flow) complete(ctx context.Context, cfg ClientConfig, code, verifier, redirectURI string) (Result, error) {
	tok, err := f.exchange(ctx, cfg, code, verifier, redirectURI)
	if err != nil {
		return Result{}, err
	}
	if !hasScope(tok.Scope, gmailModifyScope) {
		return Result{}, fmt.Errorf("%w: gmail.modify スコープが付与されませんでした", ErrDenied)
	}
	if tok.RefreshToken == "" {
		return Result{}, fmt.Errorf("%w: リフレッシュトークンが発行されませんでした", ErrDenied)
	}
	info, err := f.userinfo(ctx, tok.AccessToken)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Email:        strings.ToLower(strings.TrimSpace(info.Email)),
		Name:         info.Name,
		RefreshToken: tok.RefreshToken,
	}, nil
}

func (f *flow) authURL(cfg ClientConfig, redirectURI, state, verifier string) (string, error) {
	u, err := url.Parse(f.opts.AuthURL)
	if err != nil {
		return "", fmt.Errorf("oauth: 認可 URL が不正です: %w", err)
	}
	sum := sha256.Sum256([]byte(verifier))
	q := u.Query()
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", strings.Join(Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
	q.Set("code_challenge_method", "S256")
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ctxError は ctx の終了理由を sentinel に写像する。呼び出し側のキャンセルは ErrCancelled、
// 期限切れ（自前のタイムアウトを含む）は ErrTimeout。
func ctxError(parent, ctx context.Context) error {
	switch {
	case errors.Is(parent.Err(), context.Canceled):
		return ErrCancelled
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return ErrTimeout
	case ctx.Err() != nil:
		return ErrCancelled
	}
	return nil
}

func hasScope(granted, want string) bool {
	for _, s := range strings.Fields(granted) {
		if s == want {
			return true
		}
	}
	return false
}

// randomString は 256bit の乱数を base64url（43 文字。PKCE verifier の要件内）で返す。
func randomString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oauth: 乱数の生成に失敗しました: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
