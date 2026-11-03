package aicore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

const (
	chatgptToken   = "CHATGPT-ACCESS-TOKEN-SECRET"    //nolint:gosec // テスト用のダミー値
	chatgptRefresh = "CHATGPT-REFRESHED-TOKEN-SECRET" //nolint:gosec // テスト用のダミー値
)

// chatgptReady は ChatGPT にログインし、プロバイダ・モデルを設定して同意する。
func (f *aiFixture) chatgptReady(t *testing.T) {
	t.Helper()
	f.ChatGPT.Token = chatgptToken
	f.ChatGPT.NextToken = chatgptRefresh
	_, err := f.ai.SignInChatGPT(t.Context(), false)
	require.NoError(t, err)
	_, err = f.ai.SetAIConfig(t.Context(), "chatgpt", "gpt-plan", "https://ignored.example/v1")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "chatgpt")
	require.NoError(t, err)
}

func TestChatGPTSignInOutAndConfig(t *testing.T) {
	f := newAIFixture(t)
	cfg, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.False(t, cfg.ChatGPTSignedIn)

	f.ChatGPT.Token = chatgptToken
	cfg, err = f.ai.SignInChatGPT(t.Context(), true)
	require.NoError(t, err)
	assert.True(t, cfg.ChatGPTSignedIn)
	assert.Equal(t, "me@chatgpt.example", cfg.ChatGPTEmail)
	assert.True(t, f.ChatGPT.LastSignInOptions.DifferentAccount)
	assert.False(t, cfg.HasAPIKey, "選択中のプロバイダ（openai）の API キーは別")

	cfg, err = f.ai.SetAIConfig(t.Context(), "chatgpt", "gpt-plan", "https://ignored.example/v1")
	require.NoError(t, err)
	assert.True(t, cfg.HasAPIKey, "chatgpt では、ログイン済みが資格情報あり")
	assert.Empty(t, cfg.BaseURL, "baseUrl は openai のみ")

	b, err := json.Marshal(cfg)
	require.NoError(t, err)
	assert.NotContains(t, string(b), chatgptToken, "設定にトークンは含まれない")

	res, err := f.ai.SignOutChatGPT(t.Context())
	require.NoError(t, err)
	assert.False(t, res.Config.ChatGPTSignedIn)
	assert.False(t, res.Config.HasAPIKey)
	assert.False(t, res.RevocationConfirmed)
	f.ChatGPT.Revoked = true
	f.ChatGPT.SignedIn = true
	res, err = f.ai.SignOutChatGPT(t.Context())
	require.NoError(t, err)
	assert.True(t, res.RevocationConfirmed)
}

func TestChatGPTSignInErrorsAreMappedWithoutDetails(t *testing.T) {
	f := newAIFixture(t)
	cases := []struct {
		err  error
		code string
	}{
		{siwc.ErrCancelled, "OAUTH_CANCELLED"},
		{siwc.ErrDenied, "OAUTH_DENIED"},
		{siwc.ErrPlanNotGranted, "OAUTH_DENIED"},
		{siwc.ErrTimeout, "OAUTH_TIMEOUT"},
		{siwc.ErrBusy, "VALIDATION"},
		{siwc.ErrAccountMismatch, "VALIDATION"},
		{siwc.ErrInvalidResponse, "AI_FAILED"},
		{siwc.ErrFailed, "AI_FAILED"},
		{&net.DNSError{Err: "no such host", Name: "auth.openai.com"}, "OFFLINE"},
	}
	for _, c := range cases {
		// 下位のエラーに秘密が紛れていても、メッセージには出ない。
		f.ChatGPT.SignInErr = fmt.Errorf("%w: %s", c.err, chatgptToken)
		_, err := f.ai.SignInChatGPT(t.Context(), false)
		ae := requireError(t, c.code, err)
		assert.NotContains(t, ae.Message, chatgptToken, c.code)
		b, merr := json.Marshal(ae)
		require.NoError(t, merr)
		assert.NotContains(t, string(b), chatgptToken)
	}
}

func TestChatGPTRejectsAPIKeyOperations(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.SetAIApiKey(t.Context(), "chatgpt", "sk-x")
	requireCode(t, "VALIDATION", err)
	_, err = f.ai.ClearAIApiKey(t.Context(), "chatgpt")
	requireCode(t, "VALIDATION", err)
}

func TestChatGPTPreconditionsAndConsent(t *testing.T) {
	f := newAIFixture(t)
	s := f.open(t, "reply")
	gen := func() error {
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1")
		return err
	}
	_, err := f.ai.SetAIConfig(t.Context(), "chatgpt", "gpt-plan", "")
	require.NoError(t, err)
	requireCode(t, "AI_NOT_CONFIGURED", gen()) // 未ログイン

	f.ChatGPT.Token = chatgptToken
	_, err = f.ai.SignInChatGPT(t.Context(), false)
	require.NoError(t, err)
	requireCode(t, "AI_CONSENT_REQUIRED", gen())

	// openai への同意（同じ api.openai.com でも別のプロバイダ）では足りない。
	_, err = f.ai.RecordAIConsent(t.Context(), "openai")
	require.NoError(t, err)
	requireCode(t, "AI_CONSENT_REQUIRED", gen())

	_, err = f.ai.RecordAIConsent(t.Context(), "chatgpt")
	require.NoError(t, err)
	require.NoError(t, gen())
	assert.Equal(t, [4]string{"chatgpt", "gpt-plan", "", chatgptToken},
		[4]string{f.factory.provider, f.factory.model, f.factory.baseURL, f.factory.key})
}

func TestChatGPTUnauthorizedRefreshesOnceAndRetries(t *testing.T) {
	f := newAIFixture(t)
	f.chatgptReady(t)
	s := f.open(t, "reply")
	var mu sync.Mutex
	var keys []string
	f.Deps.AI = func(provider, model, baseURL, key string) (ai.Provider, error) {
		mu.Lock()
		keys = append(keys, key)
		mu.Unlock()
		return f.provider, nil
	}
	calls := 0
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) {
		calls++
		if calls == 1 {
			return "", fmt.Errorf("%w: HTTP 401", ai.ErrUnauthorized)
		}
		return "更新後の本文", nil
	}
	got, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1")
	require.NoError(t, err)
	assert.Contains(t, got.BodyHTML, "更新後の本文")
	assert.Equal(t, []string{chatgptToken, chatgptRefresh}, keys)
	assert.Equal(t, 1, f.ChatGPT.Refreshes)

	// 更新しても 401 なら、ループせずログインの確認を案内する。
	f.ChatGPT.Refreshes = 0
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) {
		return "", fmt.Errorf("%w: HTTP 401 %s", ai.ErrUnauthorized, chatgptRefresh)
	}
	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r2")
	ae := requireError(t, "AI_FAILED", err)
	assert.Contains(t, ae.Message, "ログイン")
	assert.NotContains(t, ae.Message, "API キー")
	assert.NotContains(t, ae.Message, chatgptRefresh)
	assert.Equal(t, 1, f.ChatGPT.Refreshes, "1 回の生成で更新は 1 度だけ")
}

func TestChatGPTReloginIsRequestedWhenRefreshFails(t *testing.T) {
	f := newAIFixture(t)
	f.chatgptReady(t)
	s := f.open(t, "reply")
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) { return "", ai.ErrUnauthorized }
	f.ChatGPT.RefreshErr = fmt.Errorf("%w: %s", siwc.ErrReauthRequired, chatgptRefresh)
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1")
	ae := requireError(t, "AI_NOT_CONFIGURED", err)
	assert.Contains(t, ae.Message, "でログイン」をやり直してください")
	assert.NotContains(t, ae.Message, chatgptRefresh)

	// 事前の更新（AccessToken）で再ログインが必要と分かった場合も同じ案内。
	f.ChatGPT.RefreshErr = nil
	f.ChatGPT.AccessErr = siwc.ErrReauthRequired
	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r2")
	requireCode(t, "AI_NOT_CONFIGURED", err)
}

func TestChatGPTPlanErrors(t *testing.T) {
	f := newAIFixture(t)
	f.chatgptReady(t)
	s := f.open(t, "reply")
	cases := []struct {
		err      error
		code     string
		contains string
	}{
		{ai.ErrUsageLimit, "AI_FAILED", "利用枠"},
		{ai.ErrNotEligible, "AI_FAILED", "ChatGPT プランを使えません"},
		{ai.ErrRateLimited, "RATE_LIMITED", ""},
	}
	for _, c := range cases {
		f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) {
			return "", fmt.Errorf("%w: %s", c.err, chatgptToken)
		}
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1")
		ae := requireError(t, c.code, err)
		assert.Contains(t, ae.Message, c.contains)
		assert.NotContains(t, ae.Message, chatgptToken)
	}
	assert.Equal(t, 0, f.ChatGPT.Refreshes, "利用枠・資格のエラーでは再試行しない")
}

// トークン・キーが、ログ・AppError（メッセージと JSON）・設定に出ないこと。
func TestSecretsNeverAppearInLogsOrErrors(t *testing.T) {
	f := newAIFixture(t)
	f.chatgptReady(t)
	s := f.open(t, "reply")
	leaks := []string{chatgptToken, chatgptRefresh, testAPIKey}

	check := func(err error) {
		t.Helper()
		require.Error(t, err)
		var ae interface{ Error() string }
		require.ErrorAs(t, err, &ae)
		b, _ := json.Marshal(err)
		for _, l := range leaks {
			assert.NotContains(t, err.Error(), l)
			assert.NotContains(t, string(b), l)
		}
	}
	for _, perr := range []error{ai.ErrUnauthorized, ai.ErrFailed, ai.ErrRateLimited, ai.ErrUsageLimit, ai.ErrNotEligible, errors.New("plain")} {
		f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) {
			return "", fmt.Errorf("%w: key=%s token=%s refresh=%s", perr, testAPIKey, chatgptToken, chatgptRefresh)
		}
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1")
		check(err)
	}
	f.Deps.AI = func(string, string, string, string) (ai.Provider, error) {
		return nil, fmt.Errorf("init failed %s %s", chatgptToken, testAPIKey)
	}
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r2")
	check(err)

	for _, p := range []string{"openai", "chatgpt", "anthropic"} {
		cfg, err := f.ai.GetAIConfig(t.Context())
		require.NoError(t, err)
		b, _ := json.Marshal(cfg)
		for _, l := range leaks {
			assert.NotContains(t, string(b), l, p)
		}
	}
	logged := f.logs.String()
	for _, l := range leaks {
		assert.NotContains(t, logged, l)
	}
}
