package oauth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/oauth"
)

const (
	testClientID     = "client-id.apps.example"
	testClientSecret = "client-secret-value"
	testCode         = "auth-code-value"
	testRefresh      = "refresh-token-value"
	gmailModify      = "https://www.googleapis.com/auth/gmail.modify"
)

// fakeGoogle はトークン・userinfo・revoke の各エンドポイントを持つ偽 Google。
// 認可画面は持たない（OpenBrowser フェイクが認可 URL を解析してコールバックを自分で叩く）。
type fakeGoogle struct {
	srv *httptest.Server

	mu            sync.Mutex
	challenge     string // 認可 URL から得た code_challenge
	tokenForms    []url.Values
	tokenStatus   int
	tokenDelay    time.Duration // トークン応答の遅延
	tokenBlock    bool          // 応答しない
	scope         string
	refreshToken  string
	userinfoEmail string
	userinfoAuth  string
	revokeForms   []url.Values
	revokeStatus  int
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	g := &fakeGoogle{
		tokenStatus:   http.StatusOK,
		scope:         gmailModify + " openid email profile",
		refreshToken:  testRefresh,
		userinfoEmail: "Taro.Yamada@Example.COM",
		revokeStatus:  http.StatusOK,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", g.handleToken)
	mux.HandleFunc("/userinfo", g.handleUserinfo)
	mux.HandleFunc("/revoke", g.handleRevoke)
	g.srv = httptest.NewServer(mux)
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGoogle) flow(open func(string) error, timeout time.Duration) oauth.Flow {
	return oauth.NewFlow(oauth.Options{
		OpenBrowser: open,
		AuthURL:     g.srv.URL + "/auth",
		TokenURL:    g.srv.URL + "/token",
		UserinfoURL: g.srv.URL + "/userinfo",
		RevokeURL:   g.srv.URL + "/revoke",
		HTTPClient:  g.srv.Client(),
		Timeout:     timeout,
	})
}

func (g *fakeGoogle) handleToken(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	g.mu.Lock()
	delay, block := g.tokenDelay, g.tokenBlock
	g.mu.Unlock()
	if block {
		<-r.Context().Done() // クライアントが諦めるまで応答しない
		return
	}
	time.Sleep(delay) // 遅いサーバーの模擬（非同期完了の待ち合わせではない）
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tokenForms = append(g.tokenForms, r.PostForm)
	f := r.PostForm
	sum := sha256.Sum256([]byte(f.Get("code_verifier")))
	valid := r.Method == http.MethodPost &&
		f.Get("grant_type") == "authorization_code" &&
		f.Get("code") == testCode &&
		f.Get("client_id") == testClientID &&
		f.Get("client_secret") == testClientSecret &&
		base64.RawURLEncoding.EncodeToString(sum[:]) == g.challenge
	if !valid || g.tokenStatus != http.StatusOK {
		status := g.tokenStatus
		if status == http.StatusOK {
			status = http.StatusBadRequest
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"`+testRefresh+`"}`)
		return
	}
	resp := map[string]any{"access_token": "access-token-value", "expires_in": 3600, "scope": g.scope, "token_type": "Bearer"}
	if g.refreshToken != "" {
		resp["refresh_token"] = g.refreshToken
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (g *fakeGoogle) handleUserinfo(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.userinfoAuth = r.Header.Get("Authorization")
	if g.userinfoAuth != "Bearer access-token-value" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"email": g.userinfoEmail, "name": "山田 太郎"})
}

func (g *fakeGoogle) handleRevoke(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.revokeForms = append(g.revokeForms, r.PostForm)
	w.WriteHeader(g.revokeStatus)
}

// browser は OpenBrowser フェイク。認可 URL を解析して code_challenge を fakeGoogle に渡し、
// behave にコールバックの叩き方を委ねる。
type browser struct {
	g       *fakeGoogle
	behave  func(t *testing.T, p authParams)
	t       *testing.T
	params  authParams
	openErr error
}

type authParams struct {
	Redirect string
	State    string
	Query    url.Values
}

func (b *browser) open(raw string) error {
	if b.openErr != nil {
		return b.openErr
	}
	u, err := url.Parse(raw)
	require.NoError(b.t, err)
	q := u.Query()
	b.params = authParams{Redirect: q.Get("redirect_uri"), State: q.Get("state"), Query: q}
	b.g.mu.Lock()
	b.g.challenge = q.Get("code_challenge")
	b.g.mu.Unlock()
	if b.behave != nil {
		b.behave(b.t, b.params)
	}
	return nil
}

// callback はリダイレクト先に GET し、ステータスと本文を返す。
func callback(t *testing.T, redirect string, q url.Values) (int, string) {
	t.Helper()
	resp, err := http.Get(redirect + "?" + q.Encode()) //nolint:gosec,noctx // テスト用のループバック
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func redirectWith(t *testing.T, p authParams, extra url.Values) {
	t.Helper()
	status, _ := callback(t, p.Redirect, extra)
	assert.Equal(t, http.StatusOK, status)
}

func assertListenerClosed(t *testing.T, redirect string) {
	t.Helper()
	u, err := url.Parse(redirect)
	require.NoError(t, err)
	conn, err := net.DialTimeout("tcp", u.Host, 2*time.Second)
	if err == nil {
		_ = conn.Close()
	}
	require.Error(t, err, "待受が閉じていない")
}
