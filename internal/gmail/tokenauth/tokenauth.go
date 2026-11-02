// Package tokenauth はアクセストークンの取得・更新と Authorization ヘッダ付与を行う RoundTripper を提供する。
package tokenauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

// expiryMargin はアクセストークンを期限切れ前に更新する余裕。
const expiryMargin = 30 * time.Second

// Config は Transport の設定。
type Config struct {
	OAuth        *oauth2.Config
	Base         http.RoundTripper // nil なら http.DefaultTransport
	TokenClient  *http.Client      // トークン更新の通信に使う（テストの差し替え用。nil なら既定）
	Store        secrets.Store     // ローテーションされたリフレッシュトークンの保存先
	AccountID    string
	RefreshToken string
	Clock        clock.Clock
	Logger       *slog.Logger
}

// Transport はリクエストごとにアクセストークンを付与する RoundTripper。
// トークンの更新はそのリクエストの ctx で行うので、呼び出しのキャンセルが更新通信にも効く。
// リフレッシュトークンがローテーションされたら secrets に保存し直す。
type Transport struct {
	cfg Config

	mu           sync.Mutex
	refreshToken string
	tok          *oauth2.Token
}

// New は Transport を作る。
func New(cfg Config) *Transport {
	if cfg.Clock == nil {
		cfg.Clock = clock.System{}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	return &Transport{cfg: cfg, refreshToken: cfg.RefreshToken}
}

// ErrRefresh はアクセストークンの取得・更新に失敗したことを表す（リクエストは Gmail へ送られていない）。
// RoundTrip が返すエラーに %w で含まれ、元の更新エラー（*oauth2.RetrieveError など）も errors.As で辿れる。
var ErrRefresh = errors.New("tokenauth: access token refresh failed")

// RoundTrip は Authorization ヘッダを付けて base に委譲する（元の req は変更しない）。
// トークンを用意できなければ ErrRefresh を含むエラーを返し、リクエストは送られない。
func (a *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := a.token(req.Context())
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, fmt.Errorf("%w: %w", ErrRefresh, err)
	}
	r := req.Clone(req.Context())
	tok.SetAuthHeader(r)
	base := a.cfg.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r)
}

// Invalidate は保持中のアクセストークンを破棄する（401 を受けたとき、次回に更新させる）。
func (a *Transport) Invalidate() {
	a.mu.Lock()
	a.tok = nil
	a.mu.Unlock()
}

func (a *Transport) token(ctx context.Context) (*oauth2.Token, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tok != nil && a.tok.AccessToken != "" && a.tok.Expiry.After(a.cfg.Clock.Now().Add(expiryMargin)) {
		return a.tok, nil
	}
	if a.cfg.TokenClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, a.cfg.TokenClient)
	}
	tok, err := a.cfg.OAuth.TokenSource(ctx, &oauth2.Token{RefreshToken: a.refreshToken}).Token()
	if err != nil {
		return nil, err
	}
	a.tok = tok
	if tok.RefreshToken != "" && tok.RefreshToken != a.refreshToken {
		a.refreshToken = tok.RefreshToken
		if err := a.cfg.Store.Set(secrets.RefreshTokenKey(a.cfg.AccountID), tok.RefreshToken); err != nil {
			// 新しいトークンは使い続けられるが、再起動後は旧トークンで更新を試みることになる。
			a.cfg.Logger.Warn("リフレッシュトークンの保存に失敗", slog.String("account_id", a.cfg.AccountID))
		}
	}
	return tok, nil
}
