package siwctest

import (
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
)

// FakeOpenAI は OpenAI の認可サーバー（discovery・JWKS・トークン・失効）。認可画面は持たず、
// OpenBrowser フェイクが認可 URL を解析してコールバックを自分で叩く。
type FakeOpenAI struct {
	T   *testing.T
	Srv *httptest.Server
	Key *rsa.PrivateKey

	Mu          sync.Mutex
	NonceByCode map[string]string
	Challenge   map[string]string // code → code_challenge
	AuthQueries []url.Values
	TokenForms  []url.Values
	RevokeForms []url.Values
	CodeSeq     int
	RefreshSeq  int

	// 振る舞いの調整。
	Subject        string
	Email          string
	Scope          string // トークン応答の scope
	AccessToken    string
	IDTokenAud     string // 空なら発行された client_id
	IDTokenNonce   string // 空なら認可 URL の nonce
	IDTokenSigner  *rsa.PrivateKey
	OmitClientID   bool // コールバックに client_id を付けない
	CallbackExtras url.Values
	TokenStatus    int    // 0 なら成功
	TokenBody      string // tokenStatus 時の本文
	RefreshStatus  int
	RefreshBody    string
	RevokeStatus   int
	// revokeHook は失効の要求を受けたとき（応答の前）に client_id つきで呼ばれる。通信を止めるのに使う。
	RevokeHook   func(clientID string)
	RefreshDelay time.Duration
	ExpiresIn    int64
	OmitRefresh  bool
	// codeRefreshSuffix は認可コードの交換で返すリフレッシュトークンの末尾に付ける（同じ client_id の再ログインで、旧と新を見分ける）。
	CodeRefreshSuffix string
	IssueQueue        []string // 動的登録で発行する client_id（先頭から使う。空なら IssuedClient）
	Clk               clock.Clock
}

func NewFakeOpenAI(t *testing.T) *FakeOpenAI {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	f := &FakeOpenAI{
		T: t, Key: key, NonceByCode: map[string]string{}, Challenge: map[string]string{},
		Subject: SubjectA, Email: "me@chatgpt.example", Scope: ScopeAll, AccessToken: SecretAccess,
		ExpiresIn: 3600, RevokeStatus: http.StatusOK, IDTokenSigner: key,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", f.handleDiscovery)
	mux.HandleFunc("/jwks", f.handleJWKS)
	mux.HandleFunc("/token", f.handleToken)
	mux.HandleFunc("/revoke", f.handleRevoke)
	f.Srv = httptest.NewServer(mux)
	t.Cleanup(f.Srv.Close)
	return f
}

func (f *FakeOpenAI) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issuer":                                f.Srv.URL,
		"authorization_endpoint":                f.Srv.URL + "/authorize",
		"token_endpoint":                        f.Srv.URL + "/token",
		"revocation_endpoint":                   f.Srv.URL + "/revoke",
		"jwks_uri":                              f.Srv.URL + "/jwks",
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (f *FakeOpenAI) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
		"n": B64(f.Key.N.Bytes()), "e": B64(big.NewInt(int64(f.Key.E)).Bytes()),
	}}})
}

// RefreshTokenFor はクライアントごとに別のリフレッシュトークン（IssuedClient は SecretRefresh）。
func RefreshTokenFor(clientID string) string {
	if clientID == IssuedClient {
		return SecretRefresh
	}
	return SecretRefresh + "-" + clientID
}

func B64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// SignJWT は RS256 の JWT を作る。
func SignJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	hdr, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
	require.NoError(t, err)
	body, err := json.Marshal(claims)
	require.NoError(t, err)
	signing := B64(hdr) + "." + B64(body)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	require.NoError(t, err)
	return signing + "." + B64(sig)
}

func (f *FakeOpenAI) Now() time.Time {
	if f.Clk != nil {
		return f.Clk.Now()
	}
	return time.Now()
}

func (f *FakeOpenAI) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	f.Mu.Lock()
	form := cloneForm(r.PostForm)
	f.TokenForms = append(f.TokenForms, form)
	grant := form.Get("grant_type")
	status, body, delay := f.TokenStatus, f.TokenBody, time.Duration(0)
	if grant == "refresh_token" {
		status, body, delay = f.RefreshStatus, f.RefreshBody, f.RefreshDelay
	}
	f.Mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	if status != 0 {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
		return
	}

	f.Mu.Lock()
	defer f.Mu.Unlock()
	resp := map[string]any{"token_type": "Bearer", "expires_in": f.ExpiresIn, "scope": f.Scope}
	switch grant {
	case "authorization_code":
		nonce := f.NonceByCode[form.Get("code")]
		// PKCE を検証する。
		sum := sha256.Sum256([]byte(form.Get("code_verifier")))
		if B64(sum[:]) != f.Challenge[form.Get("code")] {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		aud, n := f.IDTokenAud, f.IDTokenNonce
		if aud == "" {
			aud = form.Get("client_id")
		}
		if n == "" {
			n = nonce
		}
		now := f.Now()
		resp["id_token"] = SignJWT(f.T, f.IDTokenSigner, map[string]any{
			"iss": f.Srv.URL, "aud": aud, "sub": f.Subject, "email": f.Email, "nonce": n,
			"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		})
		resp["access_token"] = f.AccessToken
		if !f.OmitRefresh {
			resp["refresh_token"] = RefreshTokenFor(form.Get("client_id")) + f.CodeRefreshSuffix
		}
	case "refresh_token":
		f.RefreshSeq++
		resp["access_token"] = fmt.Sprintf("%s-refreshed-%d", f.AccessToken, f.RefreshSeq)
		resp["refresh_token"] = fmt.Sprintf("%s-rotated-%d", SecretRefresh, f.RefreshSeq)
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unsupported_grant_type"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (f *FakeOpenAI) handleRevoke(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.Mu.Lock()
	f.RevokeForms = append(f.RevokeForms, cloneForm(r.PostForm))
	status, hook := f.RevokeStatus, f.RevokeHook
	f.Mu.Unlock()
	if hook != nil {
		hook(r.PostForm.Get("client_id")) // 失効の通信を止めたいテスト用（f.Mu の外で呼ぶ）
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

// Browser は OpenBrowser のフェイク。認可 URL を記録し、callback を別 goroutine で叩く。
func (f *FakeOpenAI) Browser(t *testing.T) func(string) error {
	return func(raw string) error {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		q := u.Query()
		f.Mu.Lock()
		f.AuthQueries = append(f.AuthQueries, q)
		f.CodeSeq++
		code := fmt.Sprintf("code-%d", f.CodeSeq)
		f.NonceByCode[code] = q.Get("nonce")
		f.Challenge[code] = q.Get("code_challenge")
		extras, omit := f.CallbackExtras, f.OmitClientID
		cid := q.Get("client_id")
		if cid == "dynamic_agent_client" {
			cid = IssuedClient
			if len(f.IssueQueue) > 0 {
				cid, f.IssueQueue = f.IssueQueue[0], f.IssueQueue[1:]
			}
		}
		f.Mu.Unlock()
		cb := url.Values{"code": {code}, "state": {q.Get("state")}, "scope": {ScopeAll}}
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

func (f *FakeOpenAI) LastAuth() url.Values {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	return f.AuthQueries[len(f.AuthQueries)-1]
}

func (f *FakeOpenAI) TokenFormsCopy() []url.Values {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	return append([]url.Values(nil), f.TokenForms...)
}

func (f *FakeOpenAI) RevokeFormsCopy() []url.Values {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	return append([]url.Values(nil), f.RevokeForms...)
}
