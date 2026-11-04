package siwctests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/store"
)

// v0.1.7 までの形式（アカウント 1 件・hitomado/chatgpt_token/<kind>）から、再ログインなしで移行できる。
func TestLegacySingleAccountIsMigrated(t *testing.T) {
	legacyKey := secrets.LegacyChatGPTTokenKey
	seed := func(t *testing.T, e *env, signedIn bool) {
		t.Helper()
		reg := fmt.Sprintf(`{"hostId":"urn:uuid:11111111-1111-4111-8111-111111111111","clientId":%q,"subject":%q,"email":"old@chatgpt.example","expiresAt":%d}`,
			issuedClient, subjectA, e.clk.Now().Add(time.Hour).UnixMilli())
		require.NoError(t, e.settings.SetSetting(t.Context(), store.SettingAIChatGPT, reg))
		if signedIn {
			require.NoError(t, e.sec.Set(legacyKey("access"), "LEGACY-ACCESS"))
			require.NoError(t, e.sec.Set(legacyKey("refresh"), "LEGACY-REFRESH"))
			require.NoError(t, e.sec.Set(legacyKey("id"), "LEGACY-ID"))
		}
	}

	t.Run("ログイン中", func(t *testing.T) {
		e := newEnv(t, nil, nil)
		seed(t, e, true)
		st, err := e.m.Status(t.Context())
		require.NoError(t, err)
		assert.Equal(t, siwc.Status{SignedIn: true, Email: "old@chatgpt.example", Accounts: []siwc.Account{
			{ID: issuedClient, Email: "old@chatgpt.example", SignedIn: true, Active: true},
		}}, st)

		tok, err := e.m.AccessToken(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "LEGACY-ACCESS", tok, "期限内ならそのまま使える（通信しない）")
		assert.Empty(t, e.f.tokenFormsCopy())
		for _, k := range []string{"access", "refresh", "id"} {
			_, err := e.sec.Get(legacyKey(k))
			assert.ErrorIs(t, err, secrets.ErrNotFound, "旧キーは消す: %s", k)
		}
		got, err := e.sec.Get(tokenKey("refresh"))
		require.NoError(t, err)
		assert.Equal(t, "LEGACY-REFRESH", got)

		reg := e.reg(t)
		assert.Equal(t, "urn:uuid:11111111-1111-4111-8111-111111111111", reg.HostID, "ホスト ID は引き継ぐ")
		assert.Equal(t, issuedClient, reg.Active)
		require.Len(t, reg.Accounts, 1)
		assert.Equal(t, subjectA, reg.Accounts[0].Subject)

		// 移行後に 2 つ目を足せる。
		e.f.mu.Lock()
		e.f.issueQueue = []string{secondClient}
		e.f.mu.Unlock()
		st, err = e.m.SignIn(t.Context(), siwc.SignInOptions{})
		require.NoError(t, err)
		assert.Len(t, st.Accounts, 2)
	})

	t.Run("ログアウト済み", func(t *testing.T) {
		e := newEnv(t, nil, nil)
		seed(t, e, false)
		st, err := e.m.Status(t.Context())
		require.NoError(t, err)
		assert.False(t, st.SignedIn)
		assert.Equal(t, []siwc.Account{{ID: issuedClient, Email: "old@chatgpt.example"}}, st.Accounts, "登録は残り、使用中にはしない")
		e.relogin(t)
		assert.Equal(t, "urn:uuid:11111111-1111-4111-8111-111111111111", e.f.lastAuth().Get("ext_agent_host_id"))
	})

	t.Run("移行は 1 度だけ", func(t *testing.T) {
		e := newEnv(t, nil, nil)
		seed(t, e, true)
		_, err := e.m.Status(t.Context())
		require.NoError(t, err)
		// 移行後に、同じキーへ新しい値が入っても、2 度目の移行で上書きしない。
		require.NoError(t, e.sec.Set(tokenKey("refresh"), "NEWER-REFRESH"))
		m2 := siwc.New(siwc.Options{OpenBrowser: e.f.browser(t), Secrets: e.sec, Settings: e.settings, Clock: e.clk, Issuer: e.f.srv.URL, HTTPClient: e.f.srv.Client()})
		_, err = m2.Status(t.Context())
		require.NoError(t, err)
		got, err := e.sec.Get(tokenKey("refresh"))
		require.NoError(t, err)
		assert.Equal(t, "NEWER-REFRESH", got)
	})

	t.Run("保存先の書き込みに失敗したら旧データを消さない", func(t *testing.T) {
		sec := &flakySecrets{Memory: secrets.NewMemory()}
		e := newEnv(t, sec, nil)
		seed(t, e, true)
		sec.failSet.Store(true)
		_, err := e.m.Status(t.Context())
		require.ErrorIs(t, err, siwc.ErrFailed)
		got, gerr := sec.Get(legacyKey("refresh"))
		require.NoError(t, gerr)
		assert.Equal(t, "LEGACY-REFRESH", got, "旧キーは残る（再試行できる）")

		sec.failSet.Store(false)
		st, err := e.m.Status(t.Context())
		require.NoError(t, err)
		assert.True(t, st.SignedIn)
	})
}
