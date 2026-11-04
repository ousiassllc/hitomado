package siwctests

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

// failingSettings は、あとから数えて n 回目の SetSetting を失敗させる Settings。
type failingSettings struct {
	siwc.Settings
	mu     sync.Mutex
	failOn int // 0 なら失敗させない。1 なら次の書き込みを失敗させる
}

func (s *failingSettings) failNthWrite(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failOn = n
}

func (s *failingSettings) SetSetting(ctx context.Context, key, value string) error {
	s.mu.Lock()
	fail := false
	if s.failOn > 0 {
		s.failOn--
		fail = s.failOn == 0
	}
	s.mu.Unlock()
	if fail {
		return errors.New("disk full")
	}
	return s.Settings.SetSetting(ctx, key, value)
}

func newFailingEnv(t *testing.T) (*env, *failingSettings) {
	t.Helper()
	fs := &failingSettings{}
	e := newEnv(t, nil, func(o *siwc.Options) { fs.Settings, o.Settings = o.Settings, fs })
	return e, fs
}

// 新規登録のコールバックが既存の登録と同じ client_id を返しても、sub が違えば既存の登録・トークンを上書きしない。
func TestNewRegistrationCollidingWithAnExistingClientIDOfAnotherSubjectIsRejected(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	before := e.reg(t)

	e.f.mu.Lock()
	e.f.subject = "sub-someone-else"
	e.f.accessToken = secondAccess
	e.f.codeRefreshSuffix = "-intruder"
	e.f.mu.Unlock() // 動的登録の発行は issuedClient のまま（既存と衝突する）

	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrAccountMismatch)

	assert.Equal(t, before, e.reg(t), "登録（sub・メール・使用中）は変わらない")
	for kind, want := range map[string]string{"access": secretAccess, "refresh": secretRefresh} {
		got, err := e.sec.Get(tokenKey(kind))
		require.NoError(t, err)
		assert.Equal(t, want, got, "既存アカウントの %s トークンは上書きされない", kind)
	}
	forms := e.f.revokeFormsCopy()
	require.Len(t, forms, 1, "得たトークンは失効させる")
	assert.Equal(t, secretRefresh+"-intruder", forms[0].Get("token"))
	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok)
}

// 同じ client_id・同じ sub なら同じアカウントなので、登録は 1 件のまま更新される。
func TestNewRegistrationCollidingWithAnExistingClientIDOfTheSameSubjectKeepsOneRegistration(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	st := e.signIn(t)
	assert.Len(t, st.Accounts, 1)
	assert.Len(t, e.reg(t).Accounts, 1)
}

// 登録の保存に失敗しても、再ログイン前から有効だったセッションのトークンは消えず、今回得たトークンだけを失効させる。
func TestRegistrySaveFailureOnReloginKeepsThePreviousSession(t *testing.T) {
	e, fs := newFailingEnv(t)
	e.signIn(t)
	e.f.mu.Lock()
	e.f.accessToken = secondAccess
	e.f.codeRefreshSuffix = "-new"
	e.f.mu.Unlock()

	fs.failNthWrite(2) // 1 回目はホスト ID の確認、2 回目が ID トークン検証後の登録の保存
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{AccountID: issuedClient})
	require.ErrorIs(t, err, siwc.ErrFailed)

	for kind, want := range map[string]string{"access": secretAccess, "refresh": secretRefresh} {
		got, err := e.sec.Get(tokenKey(kind))
		require.NoError(t, err, "旧セッションの %s トークンは残る", kind)
		assert.Equal(t, want, got)
	}
	forms := e.f.revokeFormsCopy()
	require.Len(t, forms, 1)
	assert.Equal(t, secretRefresh+"-new", forms[0].Get("token"), "失効させるのは今回得た新しいトークン")
	st, err := e.m.Status(t.Context())
	require.NoError(t, err)
	assert.True(t, st.SignedIn)
	tok, err := e.m.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, secretAccess, tok)
}

// 旧セッションが無かったときは、保存したトークンを残さず、得たリフレッシュトークンを失効させる。
func TestRegistrySaveFailureOnFirstLoginLeavesNoTokensAndRevokes(t *testing.T) {
	e, fs := newFailingEnv(t)
	fs.failNthWrite(2)
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrFailed)

	for _, kind := range []string{"access", "refresh", "id"} {
		_, err := e.sec.Get(tokenKey(kind))
		assert.ErrorIs(t, err, secrets.ErrNotFound, "%s トークンは残らない", kind)
	}
	forms := e.f.revokeFormsCopy()
	require.Len(t, forms, 1)
	assert.Equal(t, secretRefresh, forms[0].Get("token"))
}

// callback の client_id は資格情報ストアのキーに使うので、区切りなどを含む値は受け付けない。
func TestClientIDWithUnsafeCharactersIsRejected(t *testing.T) {
	for _, bad := range []string{"a/b", "a#0", "oaiapp x", "../x", "-lead", strings.Repeat("a", 129)} {
		t.Run(bad, func(t *testing.T) {
			e := newEnv(t, nil, nil)
			e.f.mu.Lock()
			e.f.issueQueue = []string{bad}
			e.f.mu.Unlock()

			_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
			require.ErrorIs(t, err, siwc.ErrInvalidResponse)
			assert.Empty(t, e.reg(t).Accounts)
			assert.Empty(t, e.f.tokenFormsCopy(), "コードを交換する前に拒否する")
		})
	}
}

// ログアウト済みの使用中アカウントのメールアドレスは、使用中のログイン状態として返さない（一覧には残る）。
func TestStatusOmitsTheEmailOfASignedOutActiveAccount(t *testing.T) {
	e := newEnv(t, nil, nil)
	e.signIn(t)
	_, err := e.m.SignOut(t.Context(), issuedClient)
	require.NoError(t, err)

	st, err := e.m.Status(t.Context())
	require.NoError(t, err)
	assert.False(t, st.SignedIn)
	assert.Empty(t, st.Email)
	require.Len(t, st.Accounts, 1)
	assert.Equal(t, "me@chatgpt.example", st.Accounts[0].Email)
	assert.True(t, st.Accounts[0].Active)
}

// 登録の読み書きを並行して行っても、更新が失われない（go test -race でも通る）。
func TestConcurrentRegistryAccessDoesNotLoseUpdates(t *testing.T) {
	e := twoAccounts(t, nil, nil)
	const thirdClient = "oaiapp_THIRD"
	e.f.mu.Lock()
	e.f.subject = "sub-account-c"
	e.f.issueQueue = []string{thirdClient}
	e.f.mu.Unlock()

	var wg sync.WaitGroup
	run := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	run(func() {
		_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
		assert.NoError(t, err)
	})
	for _, id := range []string{issuedClient, secondClient} {
		run(func() {
			for range 20 {
				assert.NoError(t, e.m.AckPlanNotice(t.Context(), id))
			}
		})
	}
	run(func() {
		for range 20 {
			_, err := e.m.Status(t.Context())
			assert.NoError(t, err)
		}
	})
	run(func() {
		for range 20 {
			_, err := e.m.AccessToken(t.Context())
			assert.NoError(t, err)
		}
	})
	wg.Wait()

	reg := e.reg(t)
	require.Len(t, reg.Accounts, 3, "並行した確認の記録でログインの登録が消えない")
	assert.Equal(t, thirdClient, reg.Active)
	assert.Equal(t, "done", reg.Accounts[0].PlanNotice)
	assert.Equal(t, "done", reg.Accounts[1].PlanNotice, "並行したログインの保存で確認の記録が消えない")
	assert.Equal(t, "pending", reg.Accounts[2].PlanNotice)
}
