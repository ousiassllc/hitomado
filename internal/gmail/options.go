package gmail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/time/rate"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/gmail/backoff"
	"github.com/ousiassllc/hitomado/internal/gmail/tokenauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

const (
	// gmailScope は Hitomado が要求する唯一の Gmail スコープ（読み取り・変更・送信）。
	gmailScope = "https://www.googleapis.com/auth/gmail.modify"
	// userID は認証済みユーザー自身を表す Gmail API の識別子。
	userID = "me"
	// quotaPerSecond は Gmail API のユーザーあたりクォータ（units/秒）。
	quotaPerSecond = 250
	// defaultHTTPTimeout は HTTPClient 未指定時の、通常の API 呼び出し 1 回の総時間の上限。
	// 添付の取得・アップロード（callSpec.transfer）にはこの総時間制限を掛けない。
	defaultHTTPTimeout = 2 * time.Minute
	// defaultResponseHeaderTimeout は HTTPClient 未指定時の、リクエスト送信完了から応答ヘッダーまでの待ち上限。
	// 転送系の呼び出しは総時間ではなくこちらと呼び出し側 ctx の期限で打ち切る。
	defaultResponseHeaderTimeout = 2 * time.Minute
)

// Options は NewClient の設定。HTTPClient・Endpoint・TokenURL・Sleep はテスト用の差し替え点。
type Options struct {
	Secrets    secrets.Store // リフレッシュトークンと Client Secret の取得元（必須）
	Accounts   AccountLookup // アカウント → OAuth 資格情報の参照（必須）
	HTTPClient *http.Client  // nil なら既定。Timeout は通常の API 呼び出しにだけ掛け、添付の転送系呼び出しには掛けない
	Endpoint   string        // Gmail API のベース URL の上書き（空なら https://gmail.googleapis.com/）
	TokenURL   string        // OAuth トークンエンドポイントの上書き（空なら Google の既定）
	Clock      clock.Clock   // nil なら実時間
	// Sleep はバックオフ待ち。nil なら ctx キャンセルで中断できるタイマー待ち。
	Sleep  func(ctx context.Context, d time.Duration) error
	Logger *slog.Logger // nil なら出力しない
}

// Forgetter はアカウントごとのキャッシュ（トークン・リミッタ）を破棄できるクライアント。
// NewClient の戻り値はこれも実装する。アカウント削除・再接続後に呼ぶ。
type Forgetter interface {
	Forget(accountID string)
}

type client struct {
	opts   Options
	clock  clock.Clock
	logger *slog.Logger
	sleep  func(ctx context.Context, d time.Duration) error
	// jitter は待ち時間の上限 d から実際の待ち時間を選ぶ（既定はフルジッタ）。
	jitter func(d time.Duration) time.Duration
	base   *http.Client

	mu    sync.Mutex
	accts map[string]*account
}

// account はアカウントごとに保持する Gmail サービス・リミッタ・トークン管理。
type account struct {
	svc     *gmailapi.Service // 通常の API 呼び出し（総時間の上限あり）
	xfer    *gmailapi.Service // 添付・生 MIME の転送系呼び出し（総時間の上限なし。打ち切りは ctx と応答ヘッダー待ち）
	limiter *rate.Limiter
	auth    *tokenauth.Transport
}

// NewClient は Gmail API クライアントを返す。アカウントのクライアントは初回利用時に作り、キャッシュする。
func NewClient(opts Options) (Client, error) {
	if opts.Secrets == nil {
		return nil, errors.New("gmail: Options.Secrets は必須です")
	}
	if opts.Accounts == nil {
		return nil, errors.New("gmail: Options.Accounts は必須です")
	}
	c := &client{opts: opts, clock: opts.Clock, logger: opts.Logger, sleep: opts.Sleep, jitter: backoff.FullJitter, accts: map[string]*account{}}
	if c.clock == nil {
		c.clock = clock.System{}
	}
	if c.logger == nil {
		c.logger = slog.New(slog.DiscardHandler)
	}
	if c.sleep == nil {
		c.sleep = backoff.Sleep
	}
	c.base = opts.HTTPClient
	if c.base == nil {
		c.base = &http.Client{Timeout: defaultHTTPTimeout, Transport: defaultTransport()}
	}
	return c, nil
}

// Forget は accountID のキャッシュを破棄する。次の呼び出しで資格情報を読み直す。
func (c *client) Forget(accountID string) {
	c.mu.Lock()
	delete(c.accts, accountID)
	c.mu.Unlock()
}

// account はキャッシュ済みのアカウントクライアントを返し、無ければ資格情報を解決して作る。
func (c *client) account(ctx context.Context, accountID string) (*account, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if a, ok := c.accts[accountID]; ok {
		return a, nil
	}
	a, err := c.newAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	c.accts[accountID] = a
	return a, nil
}

func (c *client) newAccount(ctx context.Context, accountID string) (*account, error) {
	clientID, rowID, err := c.opts.Accounts.OAuthCredentials(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("gmail: OAuth 資格情報の解決に失敗: %w: %w", ErrGeneric, err)
	}
	refresh, err := c.opts.Secrets.Get(secrets.RefreshTokenKey(accountID))
	if errors.Is(err, secrets.ErrNotFound) {
		return nil, fmt.Errorf("gmail: リフレッシュトークンがありません: %w", ErrNeedsReauth)
	}
	if err != nil {
		return nil, fmt.Errorf("gmail: リフレッシュトークンの取得に失敗: %w", ErrGeneric)
	}
	secret, err := c.opts.Secrets.Get(secrets.OAuthClientSecretKey(rowID))
	if err != nil {
		return nil, fmt.Errorf("gmail: Client Secret の取得に失敗: %w", ErrGeneric)
	}
	tokenURL := c.opts.TokenURL
	if tokenURL == "" {
		tokenURL = google.Endpoint.TokenURL
	}
	auth := tokenauth.New(tokenauth.Config{
		OAuth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: secret,
			Scopes:       []string{gmailScope},
			Endpoint:     oauth2.Endpoint{TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInParams},
		},
		Base:         c.base.Transport,
		TokenClient:  c.base,
		Store:        c.opts.Secrets,
		AccountID:    accountID,
		RefreshToken: refresh,
		Clock:        c.clock,
		Logger:       c.logger,
	})
	svc, err := c.newService(ctx, auth, c.base.Timeout)
	if err != nil {
		return nil, err
	}
	xfer, err := c.newService(ctx, auth, 0)
	if err != nil {
		return nil, err
	}
	return &account{svc: svc, xfer: xfer, limiter: rate.NewLimiter(quotaPerSecond, quotaPerSecond), auth: auth}, nil
}

// newService は auth 経由で Gmail API を呼ぶサービスを作る。timeout は http.Client.Timeout（0 なら総時間の上限なし）。
func (c *client) newService(ctx context.Context, auth *tokenauth.Transport, timeout time.Duration) (*gmailapi.Service, error) {
	svcOpts := []option.ClientOption{option.WithHTTPClient(&http.Client{Transport: auth, Timeout: timeout})}
	if c.opts.Endpoint != "" {
		svcOpts = append(svcOpts, option.WithEndpoint(c.opts.Endpoint))
	}
	svc, err := gmailapi.NewService(ctx, svcOpts...)
	if err != nil {
		return nil, fmt.Errorf("gmail: サービスの初期化に失敗: %w", ErrGeneric)
	}
	return svc, nil
}

// defaultTransport は既定のトランスポートに応答ヘッダー待ちの上限を足したもの。
func defaultTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = defaultResponseHeaderTimeout
	return t
}
