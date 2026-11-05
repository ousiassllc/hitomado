package aicoretests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// 2 つの ChatGPT アカウントを登録し、切り替えて下書きを作れる。片方をログアウトしても、もう片方は使える。
func TestChatGPTTwoAccountsSwitchAndGenerate(t *testing.T) {
	f := newAIFixture(t)
	s := f.open(t, "reply")
	tokenA, tokenB := "ACCOUNT-A-ACCESS-SECRET", "ACCOUNT-B-ACCESS-SECRET" //nolint:gosec // テスト用のダミー値

	f.ChatGPT.Email, f.ChatGPT.Token = "a@chatgpt.example", tokenA
	_, err := f.ai.SignInChatGPT(t.Context(), "")
	require.NoError(t, err)
	f.ChatGPT.Email, f.ChatGPT.Token = "b@chatgpt.example", tokenB
	cfg, err := f.ai.SignInChatGPT(t.Context(), "")
	require.NoError(t, err)
	require.Len(t, cfg.ChatGPTAccounts, 2)
	idA, idB := cfg.ChatGPTAccounts[0].ID, cfg.ChatGPTAccounts[1].ID
	assert.NotEqual(t, idA, idB)
	assert.Equal(t, "b@chatgpt.example", cfg.ChatGPTEmail, "ログインしたアカウントが使用中になる")
	assert.False(t, cfg.ChatGPTAccounts[0].Active)
	assert.True(t, cfg.ChatGPTAccounts[1].Active)

	_, err = f.ai.SetAIConfig(t.Context(), "chatgpt", "gpt-plan", "", "")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "chatgpt", "")
	require.NoError(t, err)
	gen := func(id string) {
		t.Helper()
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, id, "")
		require.NoError(t, err)
	}
	gen("r1")
	assert.Equal(t, tokenB, f.factory.key, "使用中のアカウント（B）のトークンで生成する")

	cfg, err = f.ai.SelectChatGPTAccount(t.Context(), idA)
	require.NoError(t, err)
	assert.Equal(t, "a@chatgpt.example", cfg.ChatGPTEmail)
	assert.True(t, cfg.ChatGPTAccounts[0].Active)
	assert.False(t, cfg.ChatGPTAccounts[1].Active)
	gen("r2")
	assert.Equal(t, tokenA, f.factory.key, "切り替えたアカウント（A）のトークンで生成する")

	// 使用中でないアカウントのログアウトは、使用中のアカウントに影響しない。
	res, err := f.ai.SignOutChatGPT(t.Context(), idB)
	require.NoError(t, err)
	assert.True(t, res.Config.ChatGPTSignedIn)
	assert.Equal(t, "a@chatgpt.example", res.Config.ChatGPTEmail)
	assert.True(t, res.Config.ChatGPTAccounts[0].SignedIn)
	assert.False(t, res.Config.ChatGPTAccounts[1].SignedIn)
	gen("r3")
	assert.Equal(t, tokenA, f.factory.key)

	// ログアウト済みのアカウントは選べない。未登録の ID は NOT_FOUND。
	_, err = f.ai.SelectChatGPTAccount(t.Context(), idB)
	ae := requireError(t, "AI_NOT_CONFIGURED", err)
	assert.Equal(t, "account_signed_out", ae.Details["reason"], "画面が文面を差し替える目印")
	assert.Equal(t, "このアカウントはログアウト済みです。「再ログイン」からもう一度ログインしてください。", ae.Message)
	assert.NotContains(t, ae.Message, "Continue with ChatGPT", "生成の前提の案内（未ログインのときのボタン名）ではない")
	_, err = f.ai.SelectChatGPTAccount(t.Context(), "oaiapp_unknown")
	requireCode(t, "NOT_FOUND", err)

	// ログイン中のアカウントの登録は消せない。ログアウト済みなら消せて、もう片方は残る。
	_, err = f.ai.RemoveChatGPTAccount(t.Context(), idA)
	requireCode(t, "VALIDATION", err)
	cfg, err = f.ai.RemoveChatGPTAccount(t.Context(), idB)
	require.NoError(t, err)
	require.Len(t, cfg.ChatGPTAccounts, 1)
	assert.Equal(t, idA, cfg.ChatGPTAccounts[0].ID)
	assert.True(t, cfg.ChatGPTSignedIn)

	// どの設定にも、どちらのアカウントのトークンも出ない。
	b, err := json.Marshal(cfg)
	require.NoError(t, err)
	assert.NotContains(t, string(b), tokenA)
	assert.NotContains(t, string(b), tokenB)
	for _, l := range []string{tokenA, tokenB} {
		assert.NotContains(t, f.logs.String(), l)
	}
}

// 「ChatGPT プランを使っています」の確認は、アカウントごとに、初めてログインしたときだけ出る（再ログインでは出ない）。
func TestChatGPTPlanNoticeIsPerAccountAndOnce(t *testing.T) {
	f := newAIFixture(t)
	pending := func(cfg dto.AIConfig) []bool {
		var out []bool
		for _, a := range cfg.ChatGPTAccounts {
			out = append(out, a.PlanNoticePending)
		}
		return out
	}

	f.ChatGPT.Email, f.ChatGPT.Token = "a@chatgpt.example", chatgptToken
	cfg, err := f.ai.SignInChatGPT(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, pending(cfg), "初めてのログインでは確認を出す")
	idA := cfg.ChatGPTAccounts[0].ID

	cfg, err = f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []bool{true}, pending(cfg), "「Got it」を押すまで出し続ける（途中で閉じても失われない）")

	cfg, err = f.ai.AckChatGPTPlanNotice(t.Context(), idA)
	require.NoError(t, err)
	assert.Equal(t, []bool{false}, pending(cfg))
	cfg, err = f.ai.AckChatGPTPlanNotice(t.Context(), idA)
	require.NoError(t, err, "何度呼んでも同じ")
	assert.Equal(t, []bool{false}, pending(cfg))

	// ログアウト→同じ登録で再ログインしても出さない。
	_, err = f.ai.SignOutChatGPT(t.Context(), idA)
	require.NoError(t, err)
	cfg, err = f.ai.SignInChatGPT(t.Context(), idA)
	require.NoError(t, err)
	assert.Equal(t, []bool{false}, pending(cfg), "再ログインでは出さない")

	// 2 つ目のアカウントは初めてなので、1 度だけ出る。1 つ目は影響を受けない。
	f.ChatGPT.Email = "b@chatgpt.example"
	cfg, err = f.ai.SignInChatGPT(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, []bool{false, true}, pending(cfg))
	idB := cfg.ChatGPTAccounts[1].ID
	cfg, err = f.ai.AckChatGPTPlanNotice(t.Context(), idB)
	require.NoError(t, err)
	assert.Equal(t, []bool{false, false}, pending(cfg))

	_, err = f.ai.AckChatGPTPlanNotice(t.Context(), "oaiapp_unknown")
	requireCode(t, "NOT_FOUND", err)
}
