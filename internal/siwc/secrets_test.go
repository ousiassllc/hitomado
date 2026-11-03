package siwc_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
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
	_, err = sec.Get(secrets.ChatGPTTokenKey("access") + "#2")
	assert.ErrorIs(t, err, secrets.ErrNotFound)

	_, err = e.m.SignOut(t.Context())
	require.NoError(t, err)
	_, err = sec.Get(secrets.ChatGPTTokenKey("access"))
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
	if key == secrets.ChatGPTTokenKey("id") || strings.HasPrefix(key, secrets.ChatGPTTokenKey("id")+"#") {
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
	forbidden := []string{secretAccess, secretRefresh, "code-1", "code_verifier"}

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
	_, err = e4.m.SignOut(t.Context())
	require.NoError(t, err)

	for _, s := range all {
		for _, bad := range forbidden {
			assert.NotContains(t, s, bad)
		}
	}
	for _, x := range []*env{e, e2, e3, e4} {
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
}

// 壊れた（断片が欠けた・形式が不正な）トークンがあっても、状態の取得（GetAIConfig の元）は失敗せず、未ログインとして扱う。
func TestCorruptStoredTokensAreTreatedAsSignedOut(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	require.NoError(t, e.sec.Set(secrets.ChatGPTTokenKey("refresh"), "chunks:3"))
	st, err := e.m.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, st.SignedIn)
	_, err = e.m.AccessToken(t.Context())
	assert.ErrorIs(t, err, siwc.ErrNotSignedIn)

	require.NoError(t, e.sec.Set(secrets.ChatGPTTokenKey("refresh"), "chunks:x"))
	st, err = e.m.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, st.SignedIn)
	e.signIn(t) // 再ログインで置き換わる
	st, _ = e.m.Status(t.Context())
	assert.True(t, st.SignedIn)
}
