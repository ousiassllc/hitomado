package siwctests

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/store"
)

func TestLongTokensAreSplitUnderTheCredentialStoreLimit(t *testing.T) {
	sec := limitedSecrets{Memory: secrets.NewMemory(), max: 2560}
	e := newEnv(t, sec, nil)
	long := strings.Repeat("A", 6500)
	e.f.accessToken = long
	e.signIn(t)

	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, long, tok)

	// 短い値へ更新したとき、古い断片が残らず、読み出しが正しい。
	e.f.mu.Lock()
	e.f.accessToken = "short"
	e.f.mu.Unlock()
	e.clk.Advance(2 * time.Hour)
	tok, err = e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "short-refreshed-1", tok)
	_, err = sec.Get(tokenKey("access") + "#2")
	assert.ErrorIs(t, err, secrets.ErrNotFound)

	_, err = e.m.SignOut(t.Context(), issuedClient)
	require.NoError(t, err)
	_, err = sec.Get(tokenKey("access"))
	assert.ErrorIs(t, err, secrets.ErrNotFound)
}

func TestOversizedIDTokenDoesNotFailSignIn(t *testing.T) {
	e := newEnv(t, failingIDSecrets{secrets.NewMemory()}, nil)
	st := e.signIn(t)
	assert.True(t, st.SignedIn, "ID トークンは任意（ヒント用）なので保存できなくてもログインは成功する")
}

// failingIDSecrets は ID トークンの保存だけ失敗する資格情報ストア。
type failingIDSecrets struct{ *secrets.Memory }

func (f failingIDSecrets) Set(key, value string) error {
	if key == tokenKey("id") || strings.HasPrefix(key, tokenKey("id")+"#") {
		return errors.New("store refuses")
	}
	return f.Memory.Set(key, value)
}

// 秘密（トークン・認可コード・リフレッシュトークン）が、ログ・エラー文面・戻り値に出ないこと。
func TestSecretsNeverAppearInLogsErrorsOrStatus(t *testing.T) {
	// サーバーが失敗の応答に、秘密・要求の値・説明文をそのまま入れてくる最悪ケース。
	hostile := func(code string) string {
		return fmt.Sprintf(`{"error":%q,"error_description":"bad %s %s code-1","detail":"%s","access_token":"%s","refresh_token":"%s"}`, code,
			secretAccess, secretRefresh, secretAccess, secretAccess, secretRefresh)
	}
	forbidden := []string{secretAccess, secretRefresh, secondAccess, refreshTokenFor(secondClient), "code-1", "code_verifier"}

	var all []string
	collect := func(label string, err error) {
		if err != nil {
			all = append(all, label+": "+err.Error(), label+" %+v: "+fmt.Sprintf("%+v", err), label+" %#v: "+fmt.Sprintf("%#v", err))
		}
	}

	e := newEnv(t, nil, nil)
	e.signIn(t)
	// 更新が敵対的な応答を返す。
	e.clk.Advance(2 * time.Hour)
	e.f.mu.Lock()
	e.f.refreshStatus, e.f.refreshBody = http.StatusInternalServerError, hostile("invalid_request")
	e.f.mu.Unlock()
	_, err := e.m.AccessToken(t.Context())
	require.Error(t, err)
	collect("refresh 500", err)
	e.f.mu.Lock()
	e.f.refreshStatus, e.f.refreshBody = http.StatusBadRequest, hostile("invalid_grant")
	e.f.mu.Unlock()
	_, err = e.m.AccessToken(t.Context())
	require.ErrorIs(t, err, siwc.ErrReauthRequired)
	collect("refresh 400", err)

	// コード交換が敵対的な応答を返す。
	e2 := newEnv(t, nil, nil)
	e2.f.tokenStatus, e2.f.tokenBody = http.StatusBadRequest, hostile("invalid_request")
	_, err = e2.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.Error(t, err)
	collect("exchange 400", err)
	e2.f.mu.Lock()
	e2.f.tokenStatus, e2.f.tokenBody = http.StatusInternalServerError, hostile("invalid_request")
	e2.f.mu.Unlock()
	_, err = e2.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.Error(t, err)
	collect("exchange 500", err)

	// 検証に失敗する ID トークン。
	e3 := newEnv(t, nil, nil)
	e3.f.idTokenNonce = "x"
	_, err = e3.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.Error(t, err)
	collect("bad id token", err)

	// 失効が敵対的な応答を返しても、ログアウトは成功し、本文はログにも出ない。
	e4 := newEnv(t, nil, nil)
	e4.signIn(t)
	e4.f.mu.Lock()
	e4.f.revokeStatus = http.StatusInternalServerError
	e4.f.mu.Unlock()
	_, err = e4.m.SignOut(t.Context(), issuedClient)
	require.NoError(t, err)

	// 複数のアカウント: 切り替え・追加・ログアウト・更新の失敗・不正な指定のどのエラーにも、どちらのアカウントのトークンも出ない。
	e5 := twoAccounts(t, nil, nil)
	_, err = e5.m.SignOut(t.Context(), "oaiapp_UNKNOWN")
	require.Error(t, err)
	collect("sign out unknown", err)
	collect("select unknown", e5.m.SelectAccount(t.Context(), "oaiapp_UNKNOWN"))
	collect("remove in use", e5.m.RemoveAccount(t.Context(), secondClient))
	e5.f.mu.Lock()
	e5.f.revokeStatus = http.StatusInternalServerError
	e5.f.mu.Unlock()
	_, err = e5.m.SignOut(t.Context(), issuedClient)
	require.NoError(t, err)
	collect("select signed out", e5.m.SelectAccount(t.Context(), issuedClient))
	e5.clk.Advance(2 * time.Hour)
	e5.f.mu.Lock()
	e5.f.refreshStatus, e5.f.refreshBody = http.StatusBadRequest, hostile("invalid_grant")
	e5.f.mu.Unlock()
	_, err = e5.m.AccessToken(t.Context())
	require.ErrorIs(t, err, siwc.ErrReauthRequired)
	collect("refresh second account", err)
	e5.f.mu.Lock()
	e5.f.subject = "sub-someone-else"
	e5.f.tokenStatus, e5.f.tokenBody = http.StatusBadRequest, hostile("invalid_request")
	e5.f.mu.Unlock()
	_, err = e5.m.SignIn(t.Context(), siwc.SignInOptions{AccountID: secondClient})
	require.Error(t, err)
	collect("relogin exchange 400", err)

	for _, s := range all {
		for _, bad := range forbidden {
			assert.NotContains(t, s, bad)
		}
	}
	for _, x := range []*env{e, e2, e3, e4, e5} {
		logs := x.logs.String()
		for _, bad := range forbidden {
			assert.NotContains(t, logs, bad)
		}
		assert.NotContains(t, logs, "id_token")
		assert.NotContains(t, logs, "eyJ", "JWT の形のものが出ない")
	}
	// Status はトークンを持たない型で、表示用のメールだけを返す。
	st, err := e.m.Status(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, fmt.Sprintf("%+v", st), secretAccess)
	st5, err := e5.m.Status(t.Context())
	require.NoError(t, err)
	for _, bad := range forbidden {
		assert.NotContains(t, fmt.Sprintf("%+v %#v", st5, st5), bad)
	}
	// 登録（app_settings）にもトークンは入らない。
	reg, _, _ := e5.settings.GetSetting(t.Context(), store.SettingAIChatGPT)
	for _, bad := range forbidden {
		assert.NotContains(t, reg, bad)
	}
}

// 壊れた（断片が欠けた・形式が不正な）トークンがあっても、状態の取得は失敗しない。トークンは使えず（未ログイン扱い）、再ログインで置き換わる。
func TestCorruptStoredTokensAreTreatedAsSignedOut(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	for _, bad := range []string{"chunks:3", "chunks:x"} {
		require.NoError(t, e.sec.Set(tokenKey("refresh"), bad))
		_, err := e.m.Status(t.Context())
		require.NoError(t, err)
		_, err = e.m.AccessToken(t.Context())
		assert.ErrorIs(t, err, siwc.ErrNotSignedIn)
	}
	e.signIn(t) // 再ログインで置き換わる
	_, err := e.m.AccessToken(t.Context())
	assert.NoError(t, err)
}

// 更新で資格情報ストアへの書き込みが失敗しても、更新前のリフレッシュトークンを先に消さない。
func TestFailedWriteDoesNotDropTheExistingRefreshToken(t *testing.T) {
	sec := &flakySecrets{Memory: secrets.NewMemory()}
	e := newEnv(t, sec, nil)
	e.signIn(t)
	e.clk.Advance(2 * time.Hour)
	sec.failSet.Store(true)
	_, err := e.m.AccessToken(t.Context())
	require.Error(t, err)
	got, gerr := sec.Get(tokenKey("refresh"))
	require.NoError(t, gerr)
	assert.Equal(t, secretRefresh, got)
}

// 更新エラーのコードは、標準の OAuth の error のほか、error.code・トップレベルの code でも判定する（公式ドキュメントの各コード）。
func TestRefreshErrorCodeShapes(t *testing.T) {
	for _, body := range []string{
		`{"error":"refresh_token_expired"}`,
		`{"error":{"code":"refresh_token_expired","message":"x"}}`,
		`{"code":"refresh_token_expired"}`,
	} {
		e := newEnv(t, nil, nil)
		e.signIn(t)
		e.clk.Advance(2 * time.Hour)
		e.f.mu.Lock()
		e.f.refreshStatus, e.f.refreshBody = http.StatusUnauthorized, body
		e.f.mu.Unlock()
		_, err := e.m.AccessToken(t.Context())
		assert.ErrorIs(t, err, siwc.ErrReauthRequired, body)
	}
}

// 前回の登録のあるログイン中に、別アカウントの登録で ChatGPT プランの利用が許可されなかったとき、ログイン中のセッションは変えない。
func TestPlanDeclinedOnAnotherAccountKeepsTheActiveSession(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	e.f.mu.Lock()
	e.f.scope = "email openid profile offline_access"
	e.f.subject = "sub-account-b"
	e.f.mu.Unlock()
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrPlanNotGranted)
	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok)
	assert.Equal(t, issuedClient, e.reg(t).first().ClientID)
	assert.Equal(t, subjectA, e.reg(t).first().Subject)
}

// flakySecrets は failSet が真の間、Set を失敗させる。
type flakySecrets struct {
	*secrets.Memory
	failSet atomic.Bool
}

func (f *flakySecrets) Set(key, value string) error {
	if f.failSet.Load() {
		return errors.New("store refuses")
	}
	return f.Memory.Set(key, value)
}

// ログイン前の登録が無い状態で ChatGPT プランが許可されなかったときだけ、次のログインのために登録を残す。
func TestPlanNotGrantedFirstTimeKeepsTheRegistrationForRetry(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.f.scope = "email openid profile offline_access"
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrPlanNotGranted)
	assert.Equal(t, issuedClient, e.reg(t).first().ClientID)
	assert.True(t, e.reg(t).first().ConsentNeeded)
}
