package siwctests

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

// テスト用の秘密らしい値。ログ・エラー・戻り値に出てはならない。
const (
	secretAccess  = "ACCESS-SECRET-VALUE-1"
	secretRefresh = "REFRESH-SECRET-VALUE-1"
	issuedClient  = "oaiapp_TESTCLIENT"
	subjectA      = "sub-account-a"
	scopeAll      = "chatgpt.tokens.use.direct email offline_access openid profile resource.invoke"
)

// memSettings は siwc.Settings のメモリ実装。
type memSettings struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *memSettings) GetSetting(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	return v, ok, nil
}

func (s *memSettings) SetSetting(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = value
	return nil
}

// limitedSecrets は Windows 資格情報マネージャーと同じ 1 件 2560 バイトの上限を持つ資格情報ストア。
type limitedSecrets struct {
	*secrets.Memory
	max int
}

func (l limitedSecrets) Set(key, value string) error {
	if len(value) > l.max {
		return fmt.Errorf("too big: %d", len(value))
	}
	return l.Memory.Set(key, value)
}

// fakeOpenAI は OpenAI の認可サーバー（discovery・JWKS・トークン・失効）。認可画面は持たず、
// OpenBrowser フェイクが認可 URL を解析してコールバックを自分で叩く。
type fakeOpenAI struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey

	mu          sync.Mutex
	nonceByCode map[string]string
	challenge   map[string]string // code → code_challenge
	authQueries []url.Values
	tokenForms  []url.Values
	revokeForms []url.Values
	codeSeq     int
	refreshSeq  int

	// 振る舞いの調整。
	subject        string
	email          string
	scope          string // トークン応答の scope
	accessToken    string
	idTokenAud     string // 空なら発行された client_id
	idTokenNonce   string // 空なら認可 URL の nonce
	idTokenSigner  *rsa.PrivateKey
	omitClientID   bool // コールバックに client_id を付けない
	callbackExtras url.Values
	tokenStatus    int    // 0 なら成功
	tokenBody      string // tokenStatus 時の本文
	refreshStatus  int
	refreshBody    string
	revokeStatus   int
	// revokeHook は失効の要求を受けたとき（応答の前）に client_id つきで呼ばれる。通信を止めるのに使う。
	revokeHook   func(clientID string)
	refreshDelay time.Duration
	expiresIn    int64
	omitRefresh  bool
	// codeRefreshSuffix は認可コードの交換で返すリフレッシュトークンの末尾に付ける（同じ client_id の再ログインで、旧と新を見分ける）。
	codeRefreshSuffix string
	issueQueue        []string // 動的登録で発行する client_id（先頭から使う。空なら issuedClient）
	clk               clock.Clock
}

func newFakeOpenAI(t *testing.T) *fakeOpenAI {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	f := &fakeOpenAI{
		t: t, key: key, nonceByCode: map[string]string{}, challenge: map[string]string{},
		subject: subjectA, email: "me@chatgpt.example", scope: scopeAll, accessToken: secretAccess,
		expiresIn: 3600, revokeStatus: http.StatusOK, idTokenSigner: key,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", f.handleDiscovery)
	mux.HandleFunc("/jwks", f.handleJWKS)
	mux.HandleFunc("/token", f.handleToken)
	mux.HandleFunc("/revoke", f.handleRevoke)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOpenAI) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                f.srv.URL,
		"authorization_endpoint":                f.srv.URL + "/authorize",
		"token_endpoint":                        f.srv.URL + "/token",
		"revocation_endpoint":                   f.srv.URL + "/revoke",
		"jwks_uri":                              f.srv.URL + "/jwks",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (f *fakeOpenAI) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
		"n": b64(f.key.N.Bytes()), "e": b64(big.NewInt(int64(f.key.E)).Bytes()),
	}}})
}

// refreshTokenFor はクライアントごとに別のリフレッシュトークン（issuedClient は secretRefresh）。
func refreshTokenFor(clientID string) string {
	if clientID == issuedClient {
		return secretRefresh
	}
	return secretRefresh + "-" + clientID
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// signJWT は RS256 の JWT を作る。
func signJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	hdr, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	require.NoError(t, err)
	body, err := json.Marshal(claims)
	require.NoError(t, err)
	signing := b64(hdr) + "." + b64(body)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	require.NoError(t, err)
	return signing + "." + b64(sig)
}

func (f *fakeOpenAI) now() time.Time {
	if f.clk != nil {
		return f.clk.Now()
	}
	return time.Now()
}

func (f *fakeOpenAI) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	form := cloneForm(r.PostForm)
	f.tokenForms = append(f.tokenForms, form)
	grant := form.Get("grant_type")
	status, body, delay := f.tokenStatus, f.tokenBody, time.Duration(0)
	if grant == "refresh_token" {
		status, body, delay = f.refreshStatus, f.refreshBody, f.refreshDelay
	}
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	if status != 0 {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	resp := map[string]any{"token_type": "Bearer", "expires_in": f.expiresIn, "scope": f.scope}
	switch grant {
	case "authorization_code":
		nonce := f.nonceByCode[form.Get("code")]
		// PKCE を検証する。
		sum := sha256.Sum256([]byte(form.Get("code_verifier")))
		if b64(sum[:]) != f.challenge[form.Get("code")] {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		aud, n := f.idTokenAud, f.idTokenNonce
		if aud == "" {
			aud = form.Get("client_id")
		}
		if n == "" {
			n = nonce
		}
		now := f.now()
		resp["id_token"] = signJWT(f.t, f.idTokenSigner, map[string]any{
			"iss": f.srv.URL, "aud": aud, "sub": f.subject, "email": f.email, "nonce": n,
			"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		})
		resp["access_token"] = f.accessToken
		if !f.omitRefresh {
			resp["refresh_token"] = refreshTokenFor(form.Get("client_id")) + f.codeRefreshSuffix
		}
	case "refresh_token":
		f.refreshSeq++
		resp["access_token"] = fmt.Sprintf("%s-refreshed-%d", f.accessToken, f.refreshSeq)
		resp["refresh_token"] = fmt.Sprintf("%s-rotated-%d", secretRefresh, f.refreshSeq)
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unsupported_grant_type"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (f *fakeOpenAI) handleRevoke(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.mu.Lock()
	f.revokeForms = append(f.revokeForms, cloneForm(r.PostForm))
	status, hook := f.revokeStatus, f.revokeHook
	f.mu.Unlock()
	if hook != nil {
		hook(r.PostForm.Get("client_id")) // 失効の通信を止めたいテスト用（f.mu の外で呼ぶ）
	}
	w.WriteHeader(status)
}

func cloneForm(v url.Values) url.Values {
	c := url.Values{}
	for k, vs := range v {
		c[k] = append([]string(nil), vs...)
	}
	return c
}

// browser は OpenBrowser のフェイク。認可 URL を記録し、callback を別 goroutine で叩く。
func (f *fakeOpenAI) browser(t *testing.T) func(string) error {
	return func(raw string) error {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		q := u.Query()
		f.mu.Lock()
		f.authQueries = append(f.authQueries, q)
		f.codeSeq++
		code := fmt.Sprintf("code-%d", f.codeSeq)
		f.nonceByCode[code] = q.Get("nonce")
		f.challenge[code] = q.Get("code_challenge")
		extras, omit := f.callbackExtras, f.omitClientID
		cid := q.Get("client_id")
		if cid == "dynamic_agent_client" {
			cid = issuedClient
			if len(f.issueQueue) > 0 {
				cid, f.issueQueue = f.issueQueue[0], f.issueQueue[1:]
			}
		}
		f.mu.Unlock()
		cb := url.Values{"code": {code}, "state": {q.Get("state")}, "scope": {scopeAll}}
		if !omit {
			cb.Set("client_id", cid)
		}
		for k, vs := range extras {
			cb[k] = vs
		}
		go func() {
			resp, err := http.Get(q.Get("redirect_uri") + "?" + cb.Encode())
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
}

func (f *fakeOpenAI) lastAuth() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authQueries[len(f.authQueries)-1]
}

func (f *fakeOpenAI) tokenFormsCopy() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.tokenForms...)
}

func (f *fakeOpenAI) revokeFormsCopy() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.revokeForms...)
}
