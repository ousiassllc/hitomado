package siwc

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

const (
	// dynamicClientID は初回の動的登録の入口（発行された client_id ではないので保存しない）。
	dynamicClientID = "dynamic_agent_client"

	// 公式ドキュメントが定める scope。ChatGPT プランの利用は chatgpt.tokens.use.direct の許可で決まる。
	scopePlan     = "chatgpt.tokens.use.direct"
	requestScopes = "openid profile email offline_access resource.invoke " + scopePlan
)

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

// pendingAuth は 1 回のログインの state・nonce・PKCE。再ログインする登録は target（account）が持つ。
type pendingAuth struct {
	state, nonce, verifier string
	redirectURI            string
}

func (m *Manager) signIn(ctx context.Context, opts SignInOptions) (Status, error) {
	hostID, err := m.ensureHostID(ctx)
	if err != nil {
		return Status{}, err
	}
	reg, err := m.loadRegistry(ctx)
	if err != nil {
		return Status{}, err
	}
	// 再ログインする登録（無ければ新規登録）。選んだ登録以外は、このログインでは読まない・変えない。
	var target account
	if opts.AccountID != "" {
		a := reg.find(opts.AccountID)
		if a == nil {
			return Status{}, ErrUnknownAccount
		}
		target = *a
	}
	prov, err := m.discover(ctx)
	if err != nil {
		return Status{}, err
	}

	var p pendingAuth
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

	// 選んだ登録の保存済みの ID トークン（ログアウトで消えていれば無い）は同じアカウントを示すヒントとして
	// OpenAI の認可エンドポイントにだけ送る。ほかの登録の ID トークンは送らない。
	hintToken := ""
	if target.ClientID != "" {
		m.refreshMu.Lock() // 更新の途中の（新旧が混ざった）値を読まない
		if t, terr := m.loadTokens(target.ClientID); terr == nil {
			hintToken = t.ID
		}
		m.refreshMu.Unlock()
	}
	authURL := m.authorizeURL(prov.Endpoint().AuthURL, p, hostID, target, hintToken)
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
	return m.complete(ctx, prov, target, p, res)
}

// complete はコードの交換・ID トークンと scope の検証・保存を行う。target は再ログインした登録（新規登録なら零値）。
// 検証に通るまで、保存済みの登録・トークン・使用中のアカウントは変えない。保存するのは、このログインのアカウントのトークンと登録だけ。
func (m *Manager) complete(ctx context.Context, prov *oidc.Provider, target account, p pendingAuth, res callbackResult) (Status, error) {
	clientID := target.ClientID
	if clientID == "" {
		// 新規登録: コールバックが発行された client_id を返す。無い・不正な値なら登録は未完了。
		// この値は資格情報ストアのキーに使うので、検証してから使う。
		if res.clientID == "" || res.clientID == dynamicClientID || !validClientID(res.clientID) {
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
	// 選んだ登録へ再ログインするときは、同じアカウントであること。
	if target.ClientID != "" && target.Subject != "" && target.Subject != idt.Subject {
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
	// 取得したトークンを保存し終えるまで、ログインの中止（ctx）で中断しない（トークンと登録が食い違わないように）。
	ctx = context.WithoutCancel(ctx)

	// 新規登録で発行された client_id が保存済みの登録と同じなら、同じアカウント（sub）であること。
	// 異なるアカウントの登録・トークンを、sub を照合せずに上書きしない。sub が未記録の登録（旧形式の移行直後など）は
	// 照合できないので、ここでは止めない。
	if target.ClientID == "" {
		reg, err := m.loadRegistry(ctx)
		if err != nil {
			return Status{}, err
		}
		if ex := reg.find(clientID); ex != nil && ex.Subject != "" && ex.Subject != idt.Subject {
			m.revokeQuietly(ctx, clientID, tr.RefreshToken)
			return Status{}, ErrClientIDCollision
		}
	}

	if !granted || tr.RefreshToken == "" {
		// ログインはできたが ChatGPT プランの利用が許可されていない。得たトークンは使わず失効させる。
		m.revokeQuietly(ctx, clientID, tr.RefreshToken)
		return Status{}, m.declinePlan(ctx, clientID, idt.Subject, claims.Email)
	}

	// 保存に失敗したときに戻せるよう、上書きされる前のトークン（再ログイン前から有効なセッション）を控える。
	prev, err := m.loadTokens(clientID)
	if err != nil {
		m.revokeQuietly(ctx, clientID, tr.RefreshToken)
		return Status{}, err
	}
	if err := m.saveTokens(clientID, tokens{Access: tr.AccessToken, Refresh: tr.RefreshToken, ID: tr.IDToken}); err != nil {
		m.restoreTokens(clientID, prev)
		m.revokeQuietly(ctx, clientID, tr.RefreshToken)
		return Status{}, err
	}
	expires := m.expiry(tr.ExpiresIn)
	if err := m.updateRegistry(ctx, func(r *registry) error {
		a := r.upsert(clientID)
		a.Subject, a.Email, a.ConsentNeeded, a.ExpiresAt = idt.Subject, claims.Email, false, expires
		if a.PlanNotice == "" {
			a.PlanNotice = noticePending // ChatGPT プランの利用が初めて有効になった
		}
		r.Active = clientID // 検証を終えたこのアカウントを使用中にする
		return nil
	}); err != nil {
		// 登録を保存できなかったので、このログインで得たトークンは使えない。旧セッションがあれば元に戻し、得たトークンは失効させる。
		m.restoreTokens(clientID, prev)
		m.revokeQuietly(ctx, clientID, tr.RefreshToken)
		return Status{}, err
	}
	return m.Status(ctx)
}

// restoreTokens はアカウントのトークンを prev（ログイン前の値。無ければ空）に戻す（refreshMu を持って呼ぶ）。
// 戻せなかったときは、新旧が混ざったトークンを残さないよう消す。戻した旧トークンが OpenAI 側でまだ有効かは分からない
// （無効なら次の更新で ErrReauthRequired になり、再ログインを案内する）。ログにトークンの値は出さない。
func (m *Manager) restoreTokens(clientID string, prev tokens) {
	if prev.Refresh != "" {
		if err := m.saveTokens(clientID, prev); err == nil {
			return
		}
		m.opts.Logger.Warn("chatgpt: could not restore the previous session tokens")
	}
	if err := m.clearTokens(clientID); err != nil {
		m.opts.Logger.Warn("chatgpt: could not clear the tokens of a login that was not saved")
	}
}

// declinePlan は ChatGPT プランの利用が許可されなかったログインの後始末をして ErrPlanNotGranted を返す（そのアカウントのロックを持って呼ぶ）。
// ログイン中のアカウント（このアカウントを含む）のセッションは変えない。ログイン中でないアカウントだけ、次のログインで
// 同じクライアントを使い同意を求め直すため、登録を残す（ほかのアカウントの登録・使用中のアカウントは変えない）。
func (m *Manager) declinePlan(ctx context.Context, clientID, subject, email string) error {
	if live, err := m.hasSession(clientID); err != nil {
		return err
	} else if live {
		return ErrPlanNotGranted
	}
	if err := m.updateRegistry(ctx, func(r *registry) error {
		a := r.upsert(clientID)
		a.Subject, a.Email, a.ConsentNeeded, a.ExpiresAt = subject, email, true, 0
		return nil
	}); err != nil {
		return err
	}
	return ErrPlanNotGranted
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
