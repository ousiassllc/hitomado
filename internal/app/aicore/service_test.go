package aicore_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

const testAPIKey = "sk-secret-test-key"

// fakeProvider は ai.Provider のフェイク。呼び出しの入力を記録し、fn で応答を差し替える。
type fakeProvider struct {
	mu      sync.Mutex
	inputs  []ai.ReplyInput
	entered chan struct{}
	fn      func(ctx context.Context, in ai.ReplyInput) (string, error)
}

func (p *fakeProvider) GenerateReply(ctx context.Context, in ai.ReplyInput) (string, error) {
	p.mu.Lock()
	p.inputs = append(p.inputs, in)
	p.mu.Unlock()
	if p.entered != nil {
		close(p.entered)
	}
	return p.fn(ctx, in)
}

type aiFixture struct {
	*composeFixture
	ai       *aicore.Service
	provider *fakeProvider
	factory  struct{ provider, model, baseURL, key string }
	logs     *bytes.Buffer
}

func newAIFixture(t *testing.T) *aiFixture {
	t.Helper()
	f := &aiFixture{composeFixture: newComposeFixture(t), logs: &bytes.Buffer{}}
	f.provider = &fakeProvider{fn: func(context.Context, ai.ReplyInput) (string, error) { return "ok", nil }}
	f.Deps.Logger = slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f.Deps.AI = func(provider, model, baseURL, key string) (ai.Provider, error) {
		f.factory.provider, f.factory.model, f.factory.baseURL, f.factory.key = provider, model, baseURL, key
		return f.provider, nil
	}
	f.svc = composecore.New((*deps.Deps)(f.Deps))
	f.ai = aicore.New((*deps.Deps)(f.Deps), f.svc)
	return f
}

// ready は設定・キー・同意をすべて済ませる。
func (f *aiFixture) ready(t *testing.T) {
	t.Helper()
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://llm.example/v1")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "openai", "https://llm.example/v1")
	require.NoError(t, err)
}

func TestAIConfigDefaultsAndRoundTrip(t *testing.T) {
	f := newAIFixture(t)
	cfg, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "chatgpt", cfg.Provider, "未保存の既定は ChatGPT プラン（#104）")
	assert.False(t, cfg.Configured)
	assert.Empty(t, cfg.Model)
	assert.False(t, cfg.HasAPIKey)
	assert.NotNil(t, cfg.APIKeyProviders)
	assert.Empty(t, cfg.APIKeyProviders)
	assert.NotNil(t, cfg.ConsentedProviders)
	assert.Empty(t, cfg.ConsentedProviders)

	cfg, err = f.ai.SetAIConfig(t.Context(), "openai", " gpt-x ", "https://llm.example/v1")
	require.NoError(t, err)
	assert.True(t, cfg.Configured)
	assert.Equal(t, "gpt-x", cfg.Model)
	assert.Equal(t, "https://llm.example/v1", cfg.BaseURL)

	cfg, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	assert.True(t, cfg.HasAPIKey)
	assert.Equal(t, []string{"openai"}, cfg.APIKeyProviders)
	got, err := f.Secrets.Get(secrets.AIAPIKeyKey("openai"))
	require.NoError(t, err)
	assert.Equal(t, testAPIKey, got)

	cfg, err = f.ai.ClearAIApiKey(t.Context(), "openai")
	require.NoError(t, err)
	assert.False(t, cfg.HasAPIKey)

	cfg, err = f.ai.SetAIConfig(t.Context(), "anthropic", "claude", "https://ignored.example")
	require.NoError(t, err)
	assert.Empty(t, cfg.BaseURL, "baseUrl は openai のみ")
}

// 保存済みのプロバイダ（anthropic・openai・chatgpt）は、既定が ChatGPT になっても変わらない（#104）。
func TestSavedProviderIsNotReplacedByTheDefault(t *testing.T) {
	for _, p := range []string{"anthropic", "openai", "chatgpt"} {
		f := newAIFixture(t)
		_, err := f.ai.SetAIConfig(t.Context(), p, "m-"+p, "")
		require.NoError(t, err)
		cfg, err := f.ai.GetAIConfig(t.Context())
		require.NoError(t, err)
		assert.Equal(t, p, cfg.Provider)
		assert.Equal(t, "m-"+p, cfg.Model)
		assert.True(t, cfg.Configured, p)
	}

	// 旧バージョンが保存した値（モデル名と Base URL つきの openai）もそのまま読む。
	f := newAIFixture(t)
	require.NoError(t, f.Store.SetSetting(t.Context(), "ai.config", `{"provider":"openai","model":"gpt-old","baseUrl":"https://llm.example/v1"}`))
	cfg, err := f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "openai", cfg.Provider)
	assert.Equal(t, "gpt-old", cfg.Model)
	assert.Equal(t, "https://llm.example/v1", cfg.BaseURL)
	assert.True(t, cfg.Configured)

	// 未知のプロバイダの保存値は、未保存と同じ既定（ChatGPT）で扱う。
	require.NoError(t, f.Store.SetSetting(t.Context(), "ai.config", `{"provider":"gemini","model":"x"}`))
	cfg, err = f.ai.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "chatgpt", cfg.Provider)
	assert.False(t, cfg.Configured)
}

func TestAIConfigValidation(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.SetAIConfig(t.Context(), "gemini", "m", "")
	requireCode(t, "VALIDATION", err)
	for _, u := range []string{"ftp://x.example", "not a url", "https://", "javascript:alert(1)"} {
		_, err = f.ai.SetAIConfig(t.Context(), "openai", "m", u)
		requireCode(t, "VALIDATION", err)
	}
	_, err = f.ai.SetAIApiKey(t.Context(), "gemini", "k")
	requireCode(t, "VALIDATION", err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", "  ")
	requireCode(t, "VALIDATION", err)
	_, err = f.ai.ClearAIApiKey(t.Context(), "gemini")
	requireCode(t, "VALIDATION", err)
	_, err = f.ai.RecordAIConsent(t.Context(), "gemini", "")
	requireCode(t, "VALIDATION", err)
}

func TestAIConsentPerProvider(t *testing.T) {
	f := newAIFixture(t)
	cfg, err := f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"anthropic"}, cfg.ConsentedProviders)
	cfg, err = f.ai.RecordAIConsent(t.Context(), "openai", "")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"anthropic", "openai"}, cfg.ConsentedProviders)
	cfg, err = f.ai.RecordAIConsent(t.Context(), "openai", "")
	require.NoError(t, err)
	assert.Len(t, cfg.ConsentedProviders, 2)
}

func TestGenerateRequiresReplySession(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	_, err := f.ai.GenerateReplyDraft(t.Context(), "missing", "r1", "")
	requireCode(t, "SESSION_NOT_FOUND", err)
	for _, s := range []string{f.openNew(t, f.a.ID).SessionID, f.open(t, "forward").SessionID} {
		_, err = f.ai.GenerateReplyDraft(t.Context(), s, "r1", "")
		requireCode(t, "VALIDATION", err)
	}
	assert.Empty(t, f.provider.inputs)
}

func TestGeneratePreconditions(t *testing.T) {
	f := newAIFixture(t)
	s := f.open(t, "reply")
	gen := func() error {
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
		return err
	}
	requireCode(t, "AI_NOT_CONFIGURED", gen())

	_, err := f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	requireCode(t, "AI_NOT_CONFIGURED", gen()) // model が空

	_, err = f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "")
	require.NoError(t, err)
	requireCode(t, "AI_CONSENT_REQUIRED", gen())

	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic", "") // 別プロバイダの同意では足りない
	require.NoError(t, err)
	requireCode(t, "AI_CONSENT_REQUIRED", gen())

	_, err = f.ai.RecordAIConsent(t.Context(), "openai", "")
	require.NoError(t, err)
	require.NoError(t, gen())
	assert.Equal(t, [4]string{"openai", "gpt-test", "", testAPIKey}, [4]string{f.factory.provider, f.factory.model, f.factory.baseURL, f.factory.key})
}

func TestGenerateBuildsEscapedHTMLAndNoSecretsInLogs(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) {
		return "こんにちは <b>&</b>\n改行です\n\n次の段落", nil
	}
	s := f.open(t, "reply")
	got, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	require.NoError(t, err)
	assert.Equal(t, "<p>こんにちは &lt;b&gt;&amp;&lt;/b&gt;<br>改行です</p><p>次の段落</p>", got.BodyHTML)
	assert.NotContains(t, got.BodyHTML, "data-hitomado")

	require.Len(t, f.provider.inputs, 1)
	in := f.provider.inputs[0]
	assert.Equal(t, f.a.DisplayName, in.AccountDisplayName)
	assert.Equal(t, "Sig A", in.SignatureText)
	require.Len(t, in.Messages, 1)
	assert.Equal(t, "Alice", in.Messages[0].From)

	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r2", "")
	require.NoError(t, err)
	logged := f.logs.String()
	assert.NotContains(t, logged, testAPIKey)
	assert.NotContains(t, logged, "こんにちは")
	assert.NotContains(t, logged, "alice@x.example")
}

func TestGenerateTruncatesContext(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	var msgs []gmail.MessageFull
	for i := range 8 {
		m := apptest.Msg(fmt.Sprintf("q%d", i), "tq", apptest.Epoch.Add(time.Duration(i)*time.Hour), "INBOX")
		m.From = model.Address{Email: "alice@x.example"}
		m.BodyHTML = "<p>" + strings.Repeat("あ", 6000) + "</p>"
		msgs = append(msgs, m)
	}
	draft := apptest.Msg("qd", "tq", apptest.Epoch.Add(9*time.Hour), "DRAFT")
	draft.BodyHTML = "<p>DRAFTBODY</p>"
	f.PutThread(t, f.a.ID, "tq", append(msgs, draft)...)
	s, err := f.svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "reply", Ref: &dto.ThreadRef{AccountID: f.a.ID, ThreadID: "tq"}})
	require.NoError(t, err)

	_, err = f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	require.NoError(t, err)
	in := f.provider.inputs[0]
	assert.LessOrEqual(t, len(in.Messages), 5)
	total := 0
	for _, m := range in.Messages {
		total += len([]rune(m.Text))
		assert.NotContains(t, m.Text, "DRAFTBODY")
	}
	assert.LessOrEqual(t, total, 20000)
	assert.NotEmpty(t, in.Messages)
}
