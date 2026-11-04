package siwctests

import (
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

// caseInsensitiveStore はキーの大文字小文字を区別しない資格情報ストア（Windows 資格情報マネージャーと同じ振る舞い）。
// secrets.Memory は区別するので、このテスト用に用意する。
type caseInsensitiveStore struct {
	mu sync.Mutex
	m  map[string]string
}

func newCaseInsensitiveStore() *caseInsensitiveStore {
	return &caseInsensitiveStore{m: map[string]string{}}
}

func (s *caseInsensitiveStore) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[strings.ToLower(key)]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return v, nil
}

func (s *caseInsensitiveStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[strings.ToLower(key)] = value
	return nil
}

func (s *caseInsensitiveStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, strings.ToLower(key))
	return nil
}

var _ secrets.Store = (*caseInsensitiveStore)(nil)

// 前提の確認: このストアは大文字小文字だけが違うキーを同じ値として扱う。
func TestCaseInsensitiveStoreTreatsCaseVariantsAsTheSameKey(t *testing.T) {
	s := newCaseInsensitiveStore()
	require.NoError(t, s.Set(secrets.ChatGPTTokenKey("oaiapp_Abc", "access"), "v"))
	got, err := s.Get(secrets.ChatGPTTokenKey("oaiapp_abc", "access"))
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

// 新規登録で、既存の登録と大文字小文字だけが違う client_id が発行されても、既存アカウントの資格情報を上書きせず拒否する。
// 資格情報キーが同じになるストア（Windows）で、別アカウントのトークンが上書きされない。得たトークンは失効させる。
func TestNewRegistrationWhoseClientIDDiffersOnlyInCaseIsRejected(t *testing.T) {
	sec := newCaseInsensitiveStore()
	e := newEnv(t, sec, nil)
	e.signIn(t) // issuedClient = oaiapp_TESTCLIENT
	before := e.reg(t)

	lower := strings.ToLower(issuedClient)
	require.NotEqual(t, issuedClient, lower)
	require.True(t, strings.EqualFold(issuedClient, lower))
	e.f.mu.Lock()
	e.f.subject = "sub-someone-else" // 別のアカウント
	e.f.accessToken = secondAccess
	e.f.codeRefreshSuffix = "-intruder"
	e.f.issueQueue = []string{lower}
	e.f.mu.Unlock()

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrClientIDCollision)

	assert.Equal(t, before, e.reg(t), "登録は増えず、変わらない")
	for kind, want := range map[string]string{"access": secretAccess, "refresh": secretRefresh} {
		got, err := sec.Get(tokenKey(kind))
		require.NoError(t, err)
		assert.Equal(t, want, got, "既存アカウントの %s トークンは上書きされない", kind)
	}
	forms := e.f.revokeFormsCopy()
	require.Len(t, forms, 1, "得たトークンは失効させる")
	assert.Equal(t, lower, forms[0].Get("client_id"))
	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok, "既存アカウントは使い続けられる")
}

// 大文字小文字だけが違い、同じ sub（同じアカウント）でも、別の登録として増やさず拒否する（同じ資格情報キーを指すため）。
func TestNewRegistrationWhoseClientIDDiffersOnlyInCaseIsRejectedEvenForTheSameSubject(t *testing.T) {
	sec := newCaseInsensitiveStore()
	e := newEnv(t, sec, nil)
	e.signIn(t)
	before := e.reg(t)
	e.f.mu.Lock()
	e.f.issueQueue = []string{strings.ToUpper(issuedClient)}
	e.f.mu.Unlock()

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrClientIDCollision)
	assert.Equal(t, before, e.reg(t))
	got, err := sec.Get(tokenKey("refresh"))
	require.NoError(t, err)
	assert.Equal(t, secretRefresh, got)
}

// 選んだ登録への再ログインで、発行された client_id が選んだものと大文字小文字だけ違っても、資格情報を書く前に拒否する。
func TestReloginWhoseClientIDDiffersOnlyInCaseIsRejectedBeforeAnythingIsWritten(t *testing.T) {
	sec := newCaseInsensitiveStore()
	e := newEnv(t, sec, nil)
	e.signIn(t)
	before := e.reg(t)
	tokenForms := len(e.f.tokenFormsCopy())
	e.f.mu.Lock()
	e.f.callbackExtras = url.Values{"client_id": {strings.ToLower(issuedClient)}}
	e.f.accessToken = secondAccess
	e.f.codeRefreshSuffix = "-new"
	e.f.mu.Unlock()

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{AccountID: issuedClient})
	require.ErrorIs(t, err, siwc.ErrInvalidResponse)

	assert.Len(t, e.f.tokenFormsCopy(), tokenForms, "コードを交換しない")
	assert.Equal(t, before, e.reg(t))
	got, err := sec.Get(tokenKey("access"))
	require.NoError(t, err)
	assert.Equal(t, secretAccess, got)
}
