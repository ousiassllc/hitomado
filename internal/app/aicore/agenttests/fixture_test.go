package agenttests

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store"
)

const (
	threadID       = "t1"
	testAPIKey     = "sk-secret-test-key"             //nolint:gosec // テスト用のダミー値
	chatgptToken   = "CHATGPT-ACCESS-TOKEN-SECRET"    //nolint:gosec // テスト用のダミー値
	chatgptRefresh = "CHATGPT-REFRESHED-TOKEN-SECRET" //nolint:gosec // テスト用のダミー値
)

// fakeProvider は ai.Provider のフェイク。判断・本文づくりの入力を記録し、fn・judgeFn で応答を差し替える。
// 要約・質問（F-17・F-18）はこのテストでは使わない。
type fakeProvider struct {
	mu          sync.Mutex
	inputs      []ai.ReplyInput
	fn          func(ctx context.Context, in ai.ReplyInput) (string, error)
	judgeInputs []ai.ReplyJudgeInput
	judgeFn     func(ctx context.Context, in ai.ReplyJudgeInput) (ai.ReplyJudgment, error)
}

func (p *fakeProvider) GenerateReply(ctx context.Context, in ai.ReplyInput) (string, error) {
	p.mu.Lock()
	p.inputs = append(p.inputs, in)
	p.mu.Unlock()
	return p.fn(ctx, in)
}

func (p *fakeProvider) SummarizeThread(context.Context, ai.ThreadInput) (string, error) {
	return "", errors.New("unused")
}

func (p *fakeProvider) AskAboutThread(context.Context, ai.ThreadQuestionInput) (string, error) {
	return "", errors.New("unused")
}

// JudgeReply は入力を記録し、judgeFn（無ければ「返信が要る・質問なし」）で応答する。
func (p *fakeProvider) JudgeReply(ctx context.Context, in ai.ReplyJudgeInput) (ai.ReplyJudgment, error) {
	p.mu.Lock()
	p.judgeInputs = append(p.judgeInputs, in)
	p.mu.Unlock()
	if p.judgeFn != nil {
		return p.judgeFn(ctx, in)
	}
	return ai.ReplyJudgment{NeedsReply: true, ReasonKind: ai.ReasonRequest, Reason: "依頼されています", Questions: []string{}}, nil
}

// aiFixture は 2 アカウントと、A のスレッド t1（alice からの 1 通）を持つ環境に、フェイクのプロバイダをつないだ aicore.Service。
type aiFixture struct {
	*apptest.Env
	ai       *aicore.Service
	provider *fakeProvider
	factory  struct{ provider, model, baseURL, key string }
	logs     *bytes.Buffer
	a, b     store.Account
}

func newAIFixture(t *testing.T) *aiFixture {
	t.Helper()
	e := apptest.NewEnv(t)
	f := &aiFixture{Env: e, logs: &bytes.Buffer{}}
	f.a = e.AddAccount(t, "me@a.example")
	f.b = e.AddAccount(t, "me@b.example")
	_, err := e.Store.SetSignature(t.Context(), f.a.ID, "<p>Sig A</p>")
	require.NoError(t, err)
	m := apptest.Msg("m1", threadID, apptest.Epoch, "INBOX")
	m.From = model.Address{Name: "Alice", Email: "alice@x.example"}
	m.Subject = "Hello"
	e.PutThread(t, f.a.ID, threadID, m)

	f.provider = &fakeProvider{fn: func(context.Context, ai.ReplyInput) (string, error) { return "ok", nil }}
	e.Deps.Logger = slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e.Deps.AI = func(provider, model, baseURL, key string) (ai.Provider, error) {
		f.factory.provider, f.factory.model, f.factory.baseURL, f.factory.key = provider, model, baseURL, key
		return f.provider, nil
	}
	f.ai = aicore.New((*deps.Deps)(e.Deps), composecore.New((*deps.Deps)(e.Deps)))
	return f
}

// ready は設定・キー・同意をすべて済ませる。
func (f *aiFixture) ready(t *testing.T) {
	t.Helper()
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "openai", "https://llm.example/v1")
	require.NoError(t, err)
}

// chatgptReady は ChatGPT にログインし、プロバイダ・モデルを設定して同意する。
func (f *aiFixture) chatgptReady(t *testing.T) {
	t.Helper()
	f.ChatGPT.Token = chatgptToken
	f.ChatGPT.NextToken = chatgptRefresh
	_, err := f.ai.SignInChatGPT(t.Context(), "")
	require.NoError(t, err)
	_, err = f.ai.SetAIConfig(t.Context(), "chatgpt", "gpt-plan", "https://ignored.example/v1", "")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "chatgpt", "")
	require.NoError(t, err)
}

// requireCode は err が want コードの apperr.Error であることを検証する。
func requireCode(t *testing.T, want string, err error) {
	t.Helper()
	require.Error(t, err)
	var ae *apperr.Error
	require.True(t, errors.As(err, &ae), "apperr.Error ではない: %v", err)
	require.Equal(t, want, ae.Code)
}
