package siwctests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/siwc/siwctest"
	"github.com/ousiassllc/hitomado/internal/store"
)

// v0.1.7 までの形式（アカウント 1 件・hitomado/chatgpt_token/<kind>）から、再ログインなしで移行できる。
func TestLegacySingleAccountIsMigrated(t *testing.T) {
	legacyKey := secrets.LegacyChatGPTTokenKey
	seed := func(t *testing.T, e *siwctest.Env, signedIn bool) {
		t.Helper()
		reg := fmt.Sprintf(`{"hostId":"urn:uuid:11111111-1111-4111-8111-111111111111","clientId":%q,"subject":%q,"email":"old@chatgpt.example","expiresAt":%d}`,
			siwctest.IssuedClient, siwctest.SubjectA, e.Clk.Now().Add(time.Hour).UnixMilli())
		require.NoError(t, e.Settings.SetSetting(t.Context(), store.SettingAIChatGPT, reg))
		if signedIn {
			require.NoError(t, e.Sec.Set(legacyKey("access"), "LEGACY-ACCESS"))
			require.NoError(t, e.Sec.Set(legacyKey("refresh"), "LEGACY-REFRESH"))
			require.NoError(t, e.Sec.Set(legacyKey("id"), "LEGACY-ID"))
		}
	}

	t.Run("ログイン中", func(t *testing.T) {
		e := siwctest.NewEnv(t, nil, nil)
		seed(t, e, true)
		st, err := e.M.Status(t.Context())
		require.NoError(t, err)
		assert.Equal(t, siwc.Status{SignedIn: true, Email: "old@chatgpt.example", Accounts: []siwc.Account{
			{ID: siwctest.IssuedClient, Email: "old@chatgpt.example", SignedIn: true, Active: true},
		}}, st, "旧版でログイン済みのアカウントには、初めてのログインの確認を出さない")

		tok, err := e.M.AccessToken(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "LEGACY-ACCESS", tok, "期限内ならそのまま使える（通信しない）")
		assert.Empty(t, e.F.TokenFormsCopy())
		for _, k := range []string{"access", "refresh", "id"} {
			_, err := e.Sec.Get(legacyKey(k))
			assert.ErrorIs(t, err, secrets.ErrNotFound, "旧キーは消す: %s", k)
		}
		got, err := e.Sec.Get(siwctest.TokenKey("refresh"))
		require.NoError(t, err)
		assert.Equal(t, "LEGACY-REFRESH", got)

		reg := e.Reg(t)
		assert.Equal(t, "urn:uuid:11111111-1111-4111-8111-111111111111", reg.HostID, "ホスト ID は引き継ぐ")
		assert.Equal(t, siwctest.IssuedClient, reg.Active)
		require.Len(t, reg.Accounts, 1)
		assert.Equal(t, siwctest.SubjectA, reg.Accounts[0].Subject)

		// 移行後に 2 つ目を足せる。
		e.F.Mu.Lock()
		e.F.IssueQueue = []string{secondClient}
		e.F.Mu.Unlock()
		st, err = e.M.SignIn(t.Context(), siwc.SignInOptions{})
		require.NoError(t, err)
		assert.Len(t, st.Accounts, 2)
	})

	t.Run("ログアウト済み", func(t *testing.T) {
		e := siwctest.NewEnv(t, nil, nil)
		seed(t, e, false)
		st, err := e.M.Status(t.Context())
		require.NoError(t, err)
		assert.False(t, st.SignedIn)
		assert.Equal(t, []siwc.Account{{ID: siwctest.IssuedClient, Email: "old@chatgpt.example"}}, st.Accounts, "登録は残り、使用中にはしない")
		assert.Equal(t, "done", e.Reg(t).First().PlanNotice, "旧版で ChatGPT プランが有効になったことがある登録（consentNeeded でない）は確認済み")
		e.Relogin(t)
		assert.Equal(t, "urn:uuid:11111111-1111-4111-8111-111111111111", e.F.LastAuth().Get("ext_agent_host_id"))
	})

	t.Run("移行は 1 度だけ", func(t *testing.T) {
		e := siwctest.NewEnv(t, nil, nil)
		seed(t, e, true)
		_, err := e.M.Status(t.Context())
		require.NoError(t, err)
		// 移行後に、同じキーへ新しい値が入っても、2 度目の移行で上書きしない。
		require.NoError(t, e.Sec.Set(siwctest.TokenKey("refresh"), "NEWER-REFRESH"))
		m2 := siwc.New(siwc.Options{OpenBrowser: e.F.Browser(t), Secrets: e.Sec, Settings: e.Settings, Clock: e.Clk, Issuer: e.F.Srv.URL, HTTPClient: e.F.Srv.Client()})
		_, err = m2.Status(t.Context())
		require.NoError(t, err)
		got, err := e.Sec.Get(siwctest.TokenKey("refresh"))
		require.NoError(t, err)
		assert.Equal(t, "NEWER-REFRESH", got)
	})

	t.Run("保存先の書き込みに失敗したら旧データを消さない", func(t *testing.T) {
		sec := &flakySecrets{Memory: secrets.NewMemory()}
		e := siwctest.NewEnv(t, sec, nil)
		seed(t, e, true)
		sec.failSet.Store(true)
		_, err := e.M.Status(t.Context())
		require.ErrorIs(t, err, siwc.ErrFailed)
		got, gerr := sec.Get(legacyKey("refresh"))
		require.NoError(t, gerr)
		assert.Equal(t, "LEGACY-REFRESH", got, "旧キーは残る（再試行できる）")

		sec.failSet.Store(false)
		st, err := e.M.Status(t.Context())
		require.NoError(t, err)
		assert.True(t, st.SignedIn)
	})

	t.Run("旧キーの削除に失敗しても、次の呼び出しで消し、アカウントは使える", func(t *testing.T) {
		sec := &flakySecrets{Memory: secrets.NewMemory()}
		e := siwctest.NewEnv(t, sec, nil)
		seed(t, e, true)
		sec.failDelete.Store(true)
		st, err := e.M.Status(t.Context())
		require.NoError(t, err, "旧キーを消せなくても、移行したアカウントは使える")
		assert.True(t, st.SignedIn)
		got, gerr := sec.Get(legacyKey("refresh"))
		require.NoError(t, gerr)
		assert.Equal(t, "LEGACY-REFRESH", got, "削除に失敗した旧キーは残る")

		// 失敗している間は、同じプロセスの次の呼び出しでも削除を試し続ける（アカウントは失わない）。
		st, err = e.M.Status(t.Context())
		require.NoError(t, err)
		assert.True(t, st.SignedIn)

		// 登録は新形式に置き換わっている。削除が直ったあとの呼び出しで旧キーを消す。
		sec.failDelete.Store(false)
		st, err = e.M.Status(t.Context())
		require.NoError(t, err)
		assert.True(t, st.SignedIn)
		for _, k := range []string{"access", "refresh", "id"} {
			_, err := sec.Get(legacyKey(k))
			assert.ErrorIs(t, err, secrets.ErrNotFound, "旧キーを消す: %s", k)
		}
		got, gerr = sec.Get(siwctest.TokenKey("refresh"))
		require.NoError(t, gerr)
		assert.Equal(t, "LEGACY-REFRESH", got, "新形式のキーは消さない")
		tok, err := e.M.AccessToken(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "LEGACY-ACCESS", tok)
	})

	t.Run("プロセスが落ちたあとの再起動でも、残った旧キーを消す", func(t *testing.T) {
		sec := &flakySecrets{Memory: secrets.NewMemory()}
		e := siwctest.NewEnv(t, sec, nil)
		seed(t, e, true)
		sec.failDelete.Store(true)
		_, err := e.M.Status(t.Context())
		require.NoError(t, err)

		sec.failDelete.Store(false)
		m2 := siwc.New(siwc.Options{OpenBrowser: e.F.Browser(t), Secrets: sec, Settings: e.Settings, Clock: e.Clk, Issuer: e.F.Srv.URL, HTTPClient: e.F.Srv.Client()})
		st, err := m2.Status(t.Context())
		require.NoError(t, err)
		assert.True(t, st.SignedIn)
		for _, k := range []string{"access", "refresh", "id"} {
			_, err := sec.Get(legacyKey(k))
			assert.ErrorIs(t, err, secrets.ErrNotFound, "旧キーを消す: %s", k)
		}
	})

	t.Run("旧キーが無い（移行済み）なら、削除を試さずエラーにもならない", func(t *testing.T) {
		sec := &flakySecrets{Memory: secrets.NewMemory()}
		e := siwctest.NewEnv(t, sec, nil)
		e.SignIn(t)
		sec.failDelete.Store(true) // 呼ばれればエラーになる。旧キーが無ければ消しに行かない。
		m2 := siwc.New(siwc.Options{OpenBrowser: e.F.Browser(t), Secrets: sec, Settings: e.Settings, Clock: e.Clk, Issuer: e.F.Srv.URL, HTTPClient: e.F.Srv.Client()})
		st, err := m2.Status(t.Context())
		require.NoError(t, err)
		assert.True(t, st.SignedIn)
		_, err = sec.Get(siwctest.TokenKey("refresh"))
		require.NoError(t, err, "新形式のキーには触れない")
	})
}

// sub が未記録の既存の登録（旧形式から移行した直後など）と同じ client_id が新規登録で発行されたら、sub を照合できないので
// 上書きせず拒否する。既存の登録・トークンは変えず、得たトークンは失効させる。
func TestNewRegistrationCollidingWithAnExistingClientIDWithoutSubjectIsRejected(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	legacy := fmt.Sprintf(`{"hostId":"urn:uuid:11111111-1111-4111-8111-111111111111","clientId":%q,"email":"old@chatgpt.example","expiresAt":%d}`,
		siwctest.IssuedClient, e.Clk.Now().Add(time.Hour).UnixMilli()) // subject なし
	require.NoError(t, e.Settings.SetSetting(t.Context(), store.SettingAIChatGPT, legacy))
	require.NoError(t, e.Sec.Set(secrets.LegacyChatGPTTokenKey("access"), "LEGACY-ACCESS"))
	require.NoError(t, e.Sec.Set(secrets.LegacyChatGPTTokenKey("refresh"), "LEGACY-REFRESH"))
	_, err := e.M.Status(t.Context()) // 新形式へ移行させる
	require.NoError(t, err)
	before := e.Reg(t)
	require.Empty(t, before.First().Subject, "前提: sub が未記録")
	e.F.Mu.Lock()
	e.F.CodeRefreshSuffix = "-new" // 動的登録の発行は siwctest.IssuedClient のまま（既存と衝突する）
	e.F.Mu.Unlock()

	_, err = e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.ErrorIs(t, err, siwc.ErrClientIDCollision)

	assert.Equal(t, before, e.Reg(t), "登録（sub が空のまま・メール・使用中）は変わらない")
	for kind, want := range map[string]string{"access": "LEGACY-ACCESS", "refresh": "LEGACY-REFRESH"} {
		got, err := e.Sec.Get(siwctest.TokenKey(kind))
		require.NoError(t, err)
		assert.Equal(t, want, got, "既存の %s トークンは上書きされない", kind)
	}
	forms := e.F.RevokeFormsCopy()
	require.Len(t, forms, 1, "得たトークンは失効させる")
	assert.Equal(t, siwctest.SecretRefresh+"-new", forms[0].Get("token"))
}

// sub が未記録の登録でも、その登録を選んで再ログインすれば（衝突ではない）通り、sub が記録される。
func TestReloginOfARegistrationWithoutSubjectRecordsIt(t *testing.T) {
	e := siwctest.NewEnv(t, nil, nil)
	legacy := `{"hostId":"urn:uuid:11111111-1111-4111-8111-111111111111","clientId":"` + siwctest.IssuedClient + `","email":"old@chatgpt.example"}`
	require.NoError(t, e.Settings.SetSetting(t.Context(), store.SettingAIChatGPT, legacy))
	require.True(t, e.Relogin(t).SignedIn)
	assert.Equal(t, siwctest.SubjectA, e.Reg(t).First().Subject)
	assert.Len(t, e.Reg(t).Accounts, 1)
}
