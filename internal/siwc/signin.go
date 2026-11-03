package siwc

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	callbackPath = "/auth/callback"

	// dynamicClientID は初回の動的登録の入口（発行された client_id ではないので保存しない）。
	dynamicClientID = "dynamic_agent_client"

	// 公式ドキュメントが定める scope。ChatGPT プランの利用は chatgpt.tokens.use.direct の許可で決まる。
	scopePlan     = "chatgpt.tokens.use.direct"
	requestScopes = "openid profile email offline_access resource.invoke " + scopePlan

	pageDone   = "ログインの操作が完了しました。このタブを閉じて Hitomado に戻ってください。"
	pageDenied = "ログインが完了しませんでした。このタブを閉じて Hitomado に戻ってください。"
)

// callbackResult はリダイレクトで受け取った結果。
type callbackResult struct {
	code     string
	clientID string // 新規登録のときに発行された client_id
	scope    string
	err      error
}

// callbackServer は 127.0.0.1 の一時ポートで 1 回だけ /auth/callback を受け取る待受。
type callbackServer struct {
	srv    *http.Server
	ln     net.Listener
	state  string
	result chan callbackResult // 容量 1。最初の有効なコールバックだけが入る

	closeOnce sync.Once
}

func startCallbackServer(state string) (*callbackServer, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("%w: 待受を開始できません", ErrFailed)
	}
	c := &callbackServer{ln: ln, state: state, result: make(chan callbackResult, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, c.handle)
	c.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = c.srv.Serve(ln) }()
	return c, nil
}

// redirectURI は認可要求と交換で使う URI。docs: localhost ではなく 127.0.0.1、パスは /auth/callback、ポートだけ可変。
func (c *callbackServer) redirectURI() string { return "http://" + c.ln.Addr().String() + callbackPath }

func (c *callbackServer) close() {
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.srv.Shutdown(ctx); err != nil {
			_ = c.srv.Close()
		}
		_ = c.ln.Close()
	})
}

func (c *callbackServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	// state が一致しないリクエストは無視する（待受は続行。第三者に認可を失敗させない）。
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(c.state)) != 1 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res := callbackResult{clientID: q.Get("client_id"), scope: q.Get("scope")}
	msg := pageDone
	switch errCode := q.Get("error"); {
	case errCode == "access_denied":
		res.err, msg = ErrDenied, pageDenied
	case errCode != "":
		res.err, msg = fmt.Errorf("%w: 認可エラー", ErrFailed), pageDenied
	case q.Get("code") == "":
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	default:
		res.code = q.Get("code")
	}
	writePage(w, msg)
	select {
	case c.result <- res:
	default: // 2 回目以降は無視する
	}
}

func writePage(w http.ResponseWriter, msg string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Connection", "close")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	// 書き込み失敗（クライアントの切断）は認可結果の受け取りに影響しない。
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><title>Hitomado</title>
<style>body{font-family:sans-serif;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;background:#f5f5f5;color:#222}main{background:#fff;padding:2rem 2.5rem;border-radius:8px;box-shadow:0 1px 4px rgba(0,0,0,.2);max-width:28rem}h1{font-size:1.2rem;margin:0 0 .75rem}p{margin:0;line-height:1.6}</style>
</head><body><main><h1>Hitomado</h1><p>%s</p></main></body></html>
`, msg)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// SignIn は Auth の実装。ブラウザでの OpenAI の公式画面（パスワード・Cookie は Hitomado が受け取らない）で
// ログインさせ、コードをトークンに交換し、ID トークンと scope を検証してから保存する。
// 検証に通るまで、保存済みの登録・トークンは変えない。
func (m *Manager) SignIn(parent context.Context, opts SignInOptions) (Status, error) {
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	m.mu.Lock()
	if m.cancelSign != nil {
		m.mu.Unlock()
		return Status{}, ErrBusy
	}
	m.cancelSign = cancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.cancelSign = nil
		m.mu.Unlock()
	}()

	st, err := m.signIn(ctx, opts)
	if err != nil {
		return Status{}, mapSignInError(ctx, err)
	}
	return st, nil
}

// mapSignInError は中止・タイムアウトを分類する（ctx の原因を見る）。
func mapSignInError(ctx context.Context, err error) error {
	cause := context.Cause(ctx)
	switch {
	case errors.Is(cause, ErrCancelled), errors.Is(cause, context.Canceled):
		return fmt.Errorf("%w", ErrCancelled)
	case errors.Is(cause, context.DeadlineExceeded) && !errors.Is(err, ErrDenied):
		return fmt.Errorf("%w", ErrTimeout)
	}
	return err
}

type pendingAuth struct {
	state, nonce, verifier string
	redirectURI            string
	clientID               string // 発行済みの client_id（新規登録は空）
}

func (m *Manager) signIn(ctx context.Context, opts SignInOptions) (Status, error) {
	reg, err := m.loadRegistration(ctx)
	if err != nil {
		return Status{}, err
	}
	reg, err = m.ensureHostID(ctx, reg)
	if err != nil {
		return Status{}, err
	}
	prov, err := m.discover(ctx)
	if err != nil {
		return Status{}, err
	}

	var p pendingAuth
	if !opts.DifferentAccount {
		p.clientID = reg.ClientID
	}
	for _, f := range []*string{&p.state, &p.nonce, &p.verifier} {
		if *f, err = randomToken(32); err != nil {
			return Status{}, fmt.Errorf("%w: 乱数を作れません", ErrFailed)
		}
	}
	cb, err := startCallbackServer(p.state)
	if err != nil {
		return Status{}, err
	}
	defer cb.close()
	p.redirectURI = cb.redirectURI()

	// 保存済みの ID トークン（ログアウトで消えていれば無い）は同じアカウントを示すヒントとして OpenAI の認可エンドポイントにだけ送る。
	hintToken := ""
	if p.clientID != "" {
		if t, terr := m.loadTokens(); terr == nil {
			hintToken = t.ID
		}
	}
	authURL := m.authorizeURL(prov.Endpoint().AuthURL, p, reg, hintToken, opts)
	if err := m.opts.OpenBrowser(authURL); err != nil {
		return Status{}, fmt.Errorf("%w: ブラウザを開けません", ErrFailed)
	}

	timer := time.NewTimer(m.opts.Timeout)
	defer timer.Stop()
	var res callbackResult
	select {
	case res = <-cb.result:
	case <-ctx.Done():
		return Status{}, ctx.Err()
	case <-timer.C:
		return Status{}, ErrTimeout
	}
	if res.err != nil {
		return Status{}, res.err
	}
	return m.complete(ctx, prov, reg, p, res, opts)
}

func (m *Manager) authorizeURL(authEndpoint string, p pendingAuth, reg registration, idTokenHint string, opts SignInOptions) string {
	clientID := p.clientID
	q := url.Values{}
	if clientID == "" {
		clientID = dynamicClientID
		// agent_name_hint は初回の動的登録だけに付ける（発行済みの client_id での再認可には付けない）。
		q.Set("agent_name_hint", AgentName)
	}
	q.Set("client_id", clientID)
	q.Set("ext_agent_host_id", reg.HostID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", p.redirectURI)
	q.Set("scope", requestScopes)
	q.Set("resource", resource)
	q.Set("state", p.state)
	q.Set("nonce", p.nonce)
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", pkceChallenge(p.verifier))
	if p.clientID != "" && !opts.DifferentAccount {
		if idTokenHint != "" {
			q.Set("id_token_hint", idTokenHint)
		}
		if reg.Email != "" {
			q.Set("login_hint", reg.Email)
		}
		if reg.ConsentNeeded {
			// 前回 ChatGPT プランの利用が許可されなかった。利用者が改めてログインを選んだので、同意を求め直す。
			q.Set("prompt", "consent")
		}
	}
	return authEndpointWithQuery(authEndpoint, q)
}

func authEndpointWithQuery(endpoint string, q url.Values) string {
	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	return endpoint + sep + q.Encode()
}

// complete はコードの交換・ID トークンと scope の検証・保存を行う。
func (m *Manager) complete(ctx context.Context, prov *oidc.Provider, reg registration, p pendingAuth, res callbackResult, opts SignInOptions) (Status, error) {
	clientID := p.clientID
	if clientID == "" {
		// 新規登録: コールバックが発行された client_id を返す。無ければ登録は未完了。
		if res.clientID == "" || res.clientID == dynamicClientID {
			return Status{}, fmt.Errorf("%w: クライアント登録が完了していません", ErrInvalidResponse)
		}
		clientID = res.clientID
	} else if res.clientID != "" && res.clientID != clientID {
		// 再認可で別の client_id が返ったら、選んでいたアカウントの登録を置き換えずに拒否する。
		return Status{}, fmt.Errorf("%w: クライアント ID が一致しません", ErrInvalidResponse)
	}

	tr, err := m.requestTokens(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {res.code},
		"code_verifier": {p.verifier},
		"redirect_uri":  {p.redirectURI},
		"resource":      {resource},
	})
	if err != nil {
		var se *serverError
		if errors.As(err, &se) && se.Code == "invalid_grant" {
			return Status{}, fmt.Errorf("%w: 認可コードが使えません。もう一度お試しください", ErrInvalidResponse)
		}
		return Status{}, err
	}
	if tr.IDToken == "" {
		return Status{}, fmt.Errorf("%w: ID トークンがありません", ErrInvalidResponse)
	}
	idt, err := prov.Verifier(&oidc.Config{ClientID: clientID, Now: m.clk.Now}).Verify(oidc.ClientContext(ctx, m.hc), tr.IDToken)
	if err != nil {
		// 検証の詳細（トークンの中身を含み得る）は載せない。
		return Status{}, fmt.Errorf("%w: ID トークンが不正です", ErrInvalidResponse)
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(p.nonce)) != 1 || idt.Subject == "" {
		return Status{}, fmt.Errorf("%w: ID トークンが不正です", ErrInvalidResponse)
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := idt.Claims(&claims); err != nil {
		return Status{}, fmt.Errorf("%w: ID トークンが不正です", ErrInvalidResponse)
	}
	// 保存済みの登録へ再ログインするときは、同じアカウントであること。
	if p.clientID != "" && reg.Subject != "" && reg.Subject != idt.Subject {
		m.revokeQuietly(ctx, clientID, tr.RefreshToken)
		return Status{}, ErrAccountMismatch
	}

	scope := tr.Scope
	if scope == "" {
		scope = res.scope
	}
	granted := slices.Contains(strings.Fields(scope), scopePlan)

	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	// 登録が置き換わる（別アカウント）ときは、後で古いセッションを失効させるため控えておく。
	old, _ := m.loadTokens()
	oldClient := reg.ClientID

	if !granted || tr.RefreshToken == "" {
		// ログインはできたが ChatGPT プランの利用が許可されていない。トークンは保存せず（失効させ）、登録だけ残す。
		m.revokeQuietly(ctx, clientID, tr.RefreshToken)
		reg.ClientID, reg.Subject, reg.Email, reg.ConsentNeeded, reg.ExpiresAt = clientID, idt.Subject, claims.Email, true, 0
		if err := m.clearTokens(); err != nil {
			return Status{}, err
		}
		if err := m.saveRegistration(ctx, reg); err != nil {
			return Status{}, err
		}
		return Status{}, ErrPlanNotGranted
	}

	if err := m.saveTokens(tokens{Access: tr.AccessToken, Refresh: tr.RefreshToken, ID: tr.IDToken}); err != nil {
		return Status{}, err
	}
	reg.ClientID, reg.Subject, reg.Email, reg.ConsentNeeded = clientID, idt.Subject, claims.Email, false
	reg.ExpiresAt = m.expiry(tr.ExpiresIn)
	if err := m.saveRegistration(ctx, reg); err != nil {
		_ = m.clearTokens()
		return Status{}, err
	}
	if oldClient != "" && oldClient != clientID && old.Refresh != "" {
		m.revokeQuietly(ctx, oldClient, old.Refresh)
	}
	return Status{SignedIn: true, Email: claims.Email}, nil
}

// revokeQuietly は失効を 1 度だけ試み、結果は問わない（検証に失敗した・不要になったトークンの後始末）。
func (m *Manager) revokeQuietly(ctx context.Context, clientID, refreshToken string) {
	if refreshToken == "" || clientID == "" {
		return
	}
	// ctx が中断済みでも後始末は短時間だけ試す。
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_ = m.revoke(rctx, clientID, refreshToken)
}
