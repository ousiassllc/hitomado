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
	"github.com/ousiassllc/hitomado/internal/siwc/siwctest"
	"github.com/ousiassllc/hitomado/internal/store"
)

func TestLongTokensAreSplitUnderTheCredentialStoreLimit(t *testing.T) {
	sec := siwctest.LimitedSecrets{Memory: secrets.NewMemory(), Max: 2560}
	e := siwctest.NewEnv(t, sec, nil)
	long := strings.Repeat("A", 6500)
	e.F.AccessToken = long
	e.SignIn(t)

	tok, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, long, tok)

	// 短い値へ更新したとき、古い断片が残らず、読み出しが正しい。
	e.F.Mu.Lock()
	e.F.AccessToken = "short"
	e.F.Mu.Unlock()
	e.Clk.Advance(2 * time.Hour)
	tok, err = e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "short-refreshed-1", tok)
	_, err = sec.Get(siwctest.TokenKey("access") + "#2")
	assert.ErrorIs(t, err, secrets.ErrNotFound)

	_, err = e.M.SignOut(t.Context(), siwctest.IssuedClient)
	require.NoError(t, err)
	_, err = sec.Get(siwctest.TokenKey("access"))
	assert.ErrorIs(t, err, secrets.ErrNotFound)
}

func TestOversizedIDTokenDoesNotFailSignIn(t *testing.T) {
	e := siwctest.NewEnv(t, failingIDSecrets{secrets.NewMemory()}, nil)
	st := e.SignIn(t)
	assert.True(t, st.SignedIn, "ID トークンは任意（ヒント用）なので保存できなくてもログインは成功する")
}

// failingIDSecrets は ID トークンの保存だけ失敗する資格情報ストア。
type failingIDSecrets struct{ *secrets.Memory }

func (f failingIDSecrets) Set(key, value string) error {
	if key == siwctest.TokenKey("id") || strings.HasPrefix(key, siwctest.TokenKey("id")+"#") {
		return errors.New("store refuses")
	}
	return f.Memory.Set(key, value)
}

// 秘密（トークン・認可コード・リフレッシュトークン）が、ログ・エラー文面・戻り値に出ないこと。
func TestSecretsNeverAppearInLogsErrorsOrStatus(t *testing.T) {
	// サーバーが失敗の応答に、秘密・要求の値・説明文をそのまま入れてくる最悪ケース。
	hostile := func(code string) string {
		return fmt.Sprintf(`{"error":%q,"error_description":"bad %s %s code-1","detail":"%s","access_token":"%s","refresh_token":"%s"}`, code,
			siwctest.SecretAccess, siwctest.SecretRefresh, siwctest.SecretAccess, siwctest.SecretAccess, siwctest.SecretRefresh)
	}
	forbidden := []string{siwctest.SecretAccess, siwctest.SecretRefresh, secondAccess, siwctest.RefreshTokenFor(secondClient), "code-1", "code_verifier"}

	var all []string
	collect := func(label string, err error) {
		if err != nil {
			all = append(all, label+": "+err.Error(), label+" %+v: "+fmt.Sprintf("%+v", err), label+" %#v: "+fmt.Sprintf("%#v", err))
		}
	}

	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)
	// 更新が敵対的な応答を返す。
	e.Clk.Advance(2 * time.Hour)
	e.F.Mu.Lock()
	e.F.RefreshStatus, e.F.RefreshBody = http.StatusInternalServerError, hostile("invalid_request")
	e.F.Mu.Unlock()
	_, err := e.M.AccessToken(t.Context())
	require.Error(t, err)
	collect("refresh 500", err)
	e.F.Mu.Lock()
	e.F.RefreshStatus, e.F.RefreshBody = http.StatusBadRequest, hostile("invalid_grant")
	e.F.Mu.Unlock()
	_, err = e.M.AccessToken(t.Context())
	require.ErrorIs(t, err, siwc.ErrReauthRequired)
	collect("refresh 400", err)

	// コード交換が敵対的な応答を返す。
	e2 := siwctest.NewEnv(t, nil, nil)
	e2.F.TokenStatus, e2.F.TokenBody = http.StatusBadRequest, hostile("invalid_request")
	_, err = e2.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.Error(t, err)
	collect("exchange 400", err)
	e2.F.Mu.Lock()
	e2.F.TokenStatus, e2.F.TokenBody = http.StatusInternalServerError, hostile("invalid_request")
	e2.F.Mu.Unlock()
	_, err = e2.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.Error(t, err)
	collect("exchange 500", err)

	// 検証に失敗する ID トークン。
	e3 := siwctest.NewEnv(t, nil, nil)
	e3.F.IDTokenNonce = "x"
	_, err = e3.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.Error(t, err)
	collect("bad id token", err)

	// 失効が敵対的な応答を返しても、ログアウトは成功し、本文はログにも出ない。
	e4 := siwctest.NewEnv(t, nil, nil)
	e4.SignIn(t)
	e4.F.Mu.Lock()
	e4.F.RevokeStatus = http.StatusInternalServerError
	e4.F.Mu.Unlock()
	_, err = e4.M.SignOut(t.Context(), siwctest.IssuedClient)
	require.NoError(t, err)

	// 複数のアカウント: 切り替え・追加・ログアウト・更新の失敗・不正な指定のどのエラーにも、どちらのアカウントのトークンも出ない。
	e5 := twoAccounts(t, nil, nil)
	_, err = e5.M.SignOut(t.Context(), "oaiapp_UNKNOWN")
	require.Error(t, err)
	collect("sign out unknown", err)
	collect("select unknown", e5.M.SelectAccount(t.Context(), "oaiapp_UNKNOWN"))
	collect("remove in use", e5.M.RemoveAccount(t.Context(), secondClient))
	e5.F.Mu.Lock()
	e5.F.RevokeStatus = http.StatusInternalServerError
	e5.F.Mu.Unlock()
	_, err = e5.M.SignOut(t.Context(), siwctest.IssuedClient)
	require.NoError(t, err)
	collect("select signed out", e5.M.SelectAccount(t.Context(), siwctest.IssuedClient))
	e5.Clk.Advance(2 * time.Hour)
	e5.F.Mu.Lock()
	e5.F.RefreshStatus, e5.F.RefreshBody = http.StatusBadRequest, hostile("invalid_grant")
	e5.F.Mu.Unlock()
	_, err = e5.M.AccessToken(t.Context())
	require.ErrorIs(t, err, siwc.ErrReauthRequired)
	collect("refresh second account", err)
	e5.F.Mu.Lock()
	e5.F.Subject = "sub-someone-else"
	e5.F.TokenStatus, e5.F.TokenBody = http.StatusBadRequest, hostile("invalid_request")
	e5.F.Mu.Unlock()
	_, err = e5.M.SignIn(t.Context(), siwc.SignInOptions{AccountID: secondClient})
	require.Error(t, err)
	collect("relogin exchange 400", err)

	for _, s := range all {
		for _, bad := range forbidden {
			assert.NotContains(t, s, bad)
		}
	}
	for _, x := range []*siwctest.Env{e, e2, e3, e4, e5} {
		logs := x.Logs.String()
		for _, bad := range forbidden {
			assert.NotContains(t, logs, bad)
		}
		assert.NotContains(t, logs, "id_token")
		assert.NotContains(t, logs, "eyJ", "JWT の形のものが出ない")
	}
	// Status はトークンを持たない型で、表示用のメールだけを返す。
	st, err := e.M.Status(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, fmt.Sprintf("%+v", st), siwctest.SecretAccess)
	st5, err := e5.M.Status(t.Context())
	require.NoError(t, err)
	for _, bad := range forbidden {
		assert.NotContains(t, fmt.Sprintf("%+v %#v", st5, st5), bad)
	}
	// 登録（app_settings）にもトークンは入らない。
	reg, _, _ := e5.Settings.GetSetting(t.Context(), store.SettingAIChatGPT)
	for _, bad := range forbidden {
		assert.NotContains(t, reg, bad)
	}
}

// 壊れた（断片が欠けた・形式が不正な）トークンがあっても、状態の取得は失敗しない。トークンは使えず（未ログイン扱い）、再ログインで置き換わる。
func TestCorruptStoredTokensAreTreatedAsSignedOut(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)
	for _, bad := range []string{"chunks:3", "chunks:x"} {
		require.NoError(t, e.Sec.Set(siwctest.TokenKey("refresh"), bad))
		_, err := e.M.Status(t.Context())
		require.NoError(t, err)
		_, err = e.M.AccessToken(t.Context())
		assert.ErrorIs(t, err, siwc.ErrNotSignedIn)
	}
	e.SignIn(t) // 再ログインで置き換わる
	_, err := e.M.AccessToken(t.Context())
	assert.NoError(t, err)
}

// 更新で資格情報ストアへの書き込みが失敗しても、更新前のリフレッシュトークンを先に消さない。
func TestFailedWriteDoesNotDropTheExistingRefreshToken(t *testing.T) {
	sec := &flakySecrets{Memory: secrets.NewMemory()}
	e := siwctest.NewEnv(t, sec, nil)
	e.SignIn(t)
	e.Clk.Advance(2 * time.Hour)
	sec.failSet.Store(true)
	_, err := e.M.AccessToken(t.Context())
	require.Error(t, err)
	got, gerr := sec.Get(siwctest.TokenKey("refresh"))
	require.NoError(t, gerr)
	assert.Equal(t, siwctest.SecretRefresh, got)
}

// 更新エラーのコードは、標準の OAuth の error のほか、error.code・トップレベルの code でも判定する（公式ドキュメントの各コード）。
func TestRefreshErrorCodeShapes(t *testing.T) {
	for _, body := range []string{
		`{"error":"refresh_token_expired"}`,
		`{"error":{"code":"refresh_token_expired","message":"x"}}`,
		`{"code":"refresh_token_expired"}`,
	} {
		e := siwctest.NewEnv(t, nil, nil)
		e.SignIn(t)
		e.Clk.Advance(2 * time.Hour)
		e.F.Mu.Lock()
		e.F.RefreshStatus, e.F.RefreshBody = http.StatusUnauthorized, body
		e.F.Mu.Unlock()
		_, err := e.M.AccessToken(t.Context())
		assert.ErrorIs(t, err, siwc.ErrReauthRequired, body)
	}
}

// 前回の登録のあるログイン中に、別アカウントの登録で ChatGPT プランの利用が許可されなかったとき、ログイン中のセッションは変えない。
func TestPlanDeclinedOnAnotherAccountKeepsTheActiveSession(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.SignIn(t)
	e.F.Mu.Lock()
	e.F.Scope = "email openid profile offline_access"
	e.F.Subject = "sub-account-b"
	e.F.IssueQueue = []string{secondClient} // 別のアカウントは別の client_id で登録される
	e.F.Mu.Unlock()
	_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrPlanNotGranted)
	tok, err := e.M.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, siwctest.SecretAccess, tok)
	assert.Equal(t, siwctest.IssuedClient, e.Reg(t).First().ClientID)
	assert.Equal(t, siwctest.SubjectA, e.Reg(t).First().Subject)
}

// flakySecrets は failSet が真の間 Set を、failDelete が真の間 Delete を失敗させる。
type flakySecrets struct {
	*secrets.Memory
	failSet    atomic.Bool
	failDelete atomic.Bool
}

func (f *flakySecrets) Set(key, value string) error {
	if f.failSet.Load() {
		return errors.New("store refuses")
	}
	return f.Memory.Set(key, value)
}

func (f *flakySecrets) Delete(key string) error {
	if f.failDelete.Load() {
		return errors.New("store refuses to delete")
	}
	return f.Memory.Delete(key)
}

// ログイン前の登録が無い状態で ChatGPT プランが許可されなかったときだけ、次のログインのために登録を残す。
func TestPlanNotGrantedFirstTimeKeepsTheRegistrationForRetry(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	e.F.Scope = "email openid profile offline_access"
	_, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrPlanNotGranted)
	assert.Equal(t, siwctest.IssuedClient, e.Reg(t).First().ClientID)
	assert.True(t, e.Reg(t).First().ConsentNeeded)
}
