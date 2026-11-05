package siwctests

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

// hookSettings は SetSetting を差し込みで制御する Settings。
//   - armOnBrowser を立てると、次に認可 URL を開いたあと（ホスト ID の保存より後）の書き込みをすべて失敗させる。
//   - onWrite: 書き込む値を見て、書き込みの直前に呼ばれる（regMu を持った状態で呼ばれる）。
type hookSettings struct {
	siwc.Settings
	armOnBrowser atomic.Bool
	failing      atomic.Bool
	onWrite      func(value string)
}

func (s *hookSettings) SetSetting(ctx context.Context, key, value string) error {
	if s.failing.Load() {
		return errors.New("disk full")
	}
	if s.onWrite != nil {
		s.onWrite(value)
	}
	return s.Settings.SetSetting(ctx, key, value)
}

// newHookEnv は hookSettings を使う env。
func newHookEnv(t *testing.T) (*env, *hookSettings) {
	t.Helper()
	hs := &hookSettings{}
	e := newEnv(t, nil, func(o *siwc.Options) {
		hs.Settings, o.Settings = o.Settings, hs
		open := o.OpenBrowser
		o.OpenBrowser = func(u string) error {
			if hs.armOnBrowser.Load() {
				hs.failing.Store(true)
			}
			return open(u)
		}
	})
	return e, hs
}

// 新規登録のコールバックが既存の登録と同じ client_id（または大文字小文字だけが違う client_id）を返しても、sub が違えば
// 既存の登録・トークンを上書きしない。大文字小文字だけが違うときは、同じ sub でも拒否する（Windows の資格情報ストアでは
// 同じキーになるので、キーの大文字小文字を区別しない Store で確かめる）。
func TestNewRegistrationCollidingWithAnExistingClientIDIsRejected(t *testing.T) {
	for _, tc := range []struct{ name, subject, issued string }{
		{"同じ client_id・別の sub", "sub-someone-else", issuedClient},
		{"大文字小文字だけ違う・別の sub", "sub-someone-else", strings.ToLower(issuedClient)},
		{"大文字小文字だけ違う・同じ sub", subjectA, strings.ToUpper(issuedClient)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, secrets.FoldKeyCase(secrets.NewMemory()), nil)
			e.signIn(t)
			before := e.reg(t)

			e.f.mu.Lock()
			e.f.subject = tc.subject
			e.f.accessToken = secondAccess
			e.f.codeRefreshSuffix = "-intruder"
			e.f.issueQueue = []string{tc.issued} // 既存の登録と衝突する client_id を発行させる
			e.f.mu.Unlock()

			_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
			require.ErrorIs(t, err, siwc.ErrClientIDCollision)
			assert.ErrorIs(t, err, siwc.ErrAccountMismatch)

			assert.Equal(t, before, e.reg(t), "登録（sub・メール・使用中）は変わらない")
			for kind, want := range map[string]string{"access": secretAccess, "refresh": secretRefresh} {
				got, err := e.sec.Get(tokenKey(kind))
				require.NoError(t, err)
				assert.Equal(t, want, got, "既存アカウントの %s トークンは上書きされない", kind)
			}
			forms := e.f.revokeFormsCopy()
			require.Len(t, forms, 1, "得たトークンは失効させる")
			assert.Equal(t, refreshTokenFor(tc.issued)+"-intruder", forms[0].Get("token"))
			tok, err := e.m.AccessToken(t.Context())
			require.NoError(t, err)
			assert.Equal(t, secretAccess, tok)
		})
	}
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
	e, hs := newHookEnv(t)
	e.signIn(t)
	e.f.mu.Lock()
	e.f.accessToken = secondAccess
	e.f.codeRefreshSuffix = "-new"
	e.f.mu.Unlock()

	hs.armOnBrowser.Store(true) // 認可画面を開いたあとの書き込み（ID トークン検証後の登録の保存）を失敗させる
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
	e, hs := newHookEnv(t)
	hs.armOnBrowser.Store(true)
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

// callback の client_id は資格情報ストアのキーに使うので、区切り・空白・制御文字・長すぎる値は受け付けない。
func TestClientIDThatCollidesWithKeySeparatorsIsRejected(t *testing.T) {
	for name, bad := range map[string]string{
		"slash": "a/b", "hash": "a#0", "backslash": `a\b`, "space": "oaiapp x", "dotdot": "../x",
		"control": "a\x01b", "non-ascii": "oaiapp_é", "too-long": strings.Repeat("a", 129),
	} {
		t.Run(name, func(t *testing.T) {
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

// 形式が仕様書の `oaiapp_…` と違っても、キーの区切りを含まなければログインできる（許可リストでログインを塞がない）。
func TestClientIDOfUnexpectedButSafeFormatCanSignIn(t *testing.T) {
	for name, id := range map[string]string{
		"colon": "app:x", "mixed": "oaiapp_X-1.2", "at": "a@b", "leading-dash": "-lead", "max-length": strings.Repeat("a", 128),
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, nil, nil)
			e.f.mu.Lock()
			e.f.issueQueue = []string{id}
			e.f.mu.Unlock()

			st, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
			require.NoError(t, err)
			require.Len(t, st.Accounts, 1)
			assert.Equal(t, id, st.Accounts[0].ID)
			got, err := e.sec.Get(secrets.ChatGPTTokenKey(id, "access"))
			require.NoError(t, err)
			assert.Equal(t, secretAccess, got)
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

// ログインの登録の保存（読み込み→書き込み）の最中に別の更新が来ても、regMu で待たされ、古い登録で上書きして失われない。
// regMu が無ければ、書き込みの直前に走らせた確認の記録が、そのあとの古い登録の書き戻しで消える。
func TestRegistryWriteSerializesWithConcurrentUpdates(t *testing.T) {
	e, hs := newHookEnv(t)
	e.signIn(t)
	e.f.mu.Lock()
	e.f.subject = "sub-account-b"
	e.f.issueQueue = []string{secondClient}
	e.f.mu.Unlock()

	var once sync.Once
	acked := make(chan struct{})
	hs.onWrite = func(value string) {
		if !strings.Contains(value, secondClient) {
			return
		}
		once.Do(func() {
			go func() {
				assert.NoError(t, e.m.AckPlanNotice(context.Background(), issuedClient))
				close(acked)
			}()
			select {
			case <-acked: // regMu が無いとここで先に書き込みが終わる
			case <-time.After(200 * time.Millisecond): // regMu があれば、この書き込みが終わるまで待たされる
			}
		})
	}
	_, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.NoError(t, err)
	select {
	case <-acked:
	case <-time.After(5 * time.Second):
		t.Fatal("確認の記録が完了しない")
	}

	reg := e.reg(t)
	require.Len(t, reg.Accounts, 2)
	assert.Equal(t, "done", reg.Accounts[0].PlanNotice, "ログインの保存で、並行した確認の記録が失われない")
	assert.Equal(t, "pending", reg.Accounts[1].PlanNotice)
	assert.Equal(t, secondClient, reg.Active)
}
