// Package canceltests は AI 生成の中止（CancelGenerate）が、生成の登録より前に届いても効くことのテスト（#352）。
// 設定・同意・生成そのものの単体のテストは internal/app/aicore/aicoretests にある。
package canceltests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

const (
	testKey  = "sk-secret-test-key"          //nolint:gosec // テスト用のダミー値
	chatgpt  = "CHATGPT-ACCESS-TOKEN-SECRET" //nolint:gosec // テスト用のダミー値
	threadID = "t1"
)

// provider は ai.Provider のフェイク。呼ばれた回数を数え、blocked を立てると中止まで待つ。
type provider struct {
	mu      sync.Mutex
	calls   int
	blocked bool
	entered chan struct{}
}

func (p *provider) call(ctx context.Context) (string, error) {
	p.mu.Lock()
	p.calls++
	blocked, entered := p.blocked, p.entered
	p.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
	}
	if blocked {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "ok", nil
}

func (p *provider) GenerateReply(ctx context.Context, _ ai.ReplyInput) (string, error) {
	return p.call(ctx)
}

func (p *provider) SummarizeThread(ctx context.Context, _ ai.ThreadInput) (string, error) {
	return p.call(ctx)
}

func (p *provider) AskAboutThread(ctx context.Context, _ ai.ThreadQuestionInput) (string, error) {
	return p.call(ctx)
}

func (p *provider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type fixture struct {
	*apptest.Env
	ai        *aicore.Service
	svc       *composecore.Service
	provider  *provider
	sessionID string
	ref       dto.ThreadRef
}

// newFixture は 1 アカウント・受信箱の 1 通のスレッド・返信のセッションを持つ環境。opts は aicore.Service の設定。
func newFixture(t *testing.T, opts ...aicore.Option) *fixture {
	t.Helper()
	e := apptest.NewEnv(t)
	acc := e.AddAccount(t, "me@a.example")
	m := apptest.Msg("m1", threadID, apptest.Epoch, "INBOX")
	m.From = model.Address{Name: "Alice", Email: "alice@x.example"}
	e.PutThread(t, acc.ID, threadID, m)

	f := &fixture{Env: e, provider: &provider{}, ref: dto.ThreadRef{AccountID: acc.ID, ThreadID: threadID}}
	e.Deps.AI = func(_, _, _, _ string) (ai.Provider, error) { return f.provider, nil }
	f.svc = composecore.New((*deps.Deps)(e.Deps))
	f.ai = aicore.New((*deps.Deps)(e.Deps), f.svc, opts...)
	s, err := f.svc.OpenCompose(t.Context(), dto.OpenComposeRequest{Mode: "reply", Ref: &f.ref})
	require.NoError(t, err)
	f.sessionID = s.SessionID
	return f
}

// ready は OpenAI 互換のプロバイダの設定・キー・同意と、Inbox Zero（TypeSafe AI）のキー・同意を済ませる。
func (f *fixture) ready(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	_, err := f.ai.SetAIConfig(ctx, "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	for _, p := range []string{"openai", "typesafe"} {
		_, err = f.ai.SetAIApiKey(ctx, p, testKey)
		require.NoError(t, err)
	}
	_, err = f.ai.RecordAIConsent(ctx, "openai", "https://llm.example/v1")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(ctx, "typesafe", "")
	require.NoError(t, err)
}

// chatgptReady は ChatGPT にログインし、プロバイダ・モデルを設定して同意する。
func (f *fixture) chatgptReady(t *testing.T) {
	t.Helper()
	f.ChatGPT.Token = chatgpt
	_, err := f.ai.SignInChatGPT(t.Context(), "")
	require.NoError(t, err)
	_, err = f.ai.SetAIConfig(t.Context(), "chatgpt", "gpt-plan", "https://ignored.example/v1", "")
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "chatgpt", "")
	require.NoError(t, err)
}

// entries は生成の入口（返信下書き・要約・質問・Inbox Zero）。同じ requestID で呼ぶ。
func (f *fixture) entries(t *testing.T) map[string]func(id string) error {
	return map[string]func(id string) error{
		"GenerateReplyDraft": func(id string) error {
			_, err := f.ai.GenerateReplyDraft(t.Context(), f.sessionID, id, "")
			return err
		},
		"SummarizeThread": func(id string) error {
			_, err := f.ai.SummarizeThread(t.Context(), f.ref, id)
			return err
		},
		"AskAboutThread": func(id string) error {
			_, err := f.ai.AskAboutThread(t.Context(), f.ref, id, "いつまで？", nil)
			return err
		},
		"FindInboxZeroCandidates": func(id string) error {
			_, err := f.ai.FindInboxZeroCandidates(t.Context(), []dto.ThreadRef{f.ref}, id)
			return err
		},
	}
}

func requireCode(t *testing.T, want string, err error) {
	t.Helper()
	require.Error(t, err)
	var ae *apperr.Error
	require.True(t, errors.As(err, &ae), "apperr.Error ではない: %v", err)
	require.Equal(t, want, ae.Code)
}

// 生成の登録より前に届いた CancelGenerate も効く。プロバイダ（TypeSafe を含む）へは一切送らず CANCELLED。
// 中止の記録は 1 回で消費され、同じ ID でもう一度呼べば（別の実行として）通常どおり動く。
func TestCancelGenerateBeforeStart(t *testing.T) {
	f := newFixture(t)
	f.ready(t)
	for name, call := range f.entries(t) {
		t.Run(name, func(t *testing.T) {
			providerBefore, classifierBefore := f.provider.count(), f.Classifier.Calls()
			require.NoError(t, f.ai.CancelGenerate("early-"+name))
			requireCode(t, "CANCELLED", call("early-"+name))
			assert.Equal(t, providerBefore, f.provider.count(), "中止済みの実行はプロバイダへ送らない")
			assert.Equal(t, classifierBefore, f.Classifier.Calls(), "中止済みの実行は TypeSafe へ送らない")

			require.NoError(t, call("early-"+name), "記録は消費済みなので、同じ ID の次の実行は止めない")
		})
	}
}

// ChatGPT のトークン更新の最中に CancelGenerate が届くと、更新が中断され CANCELLED。プロバイダへは送らない。
func TestCancelGenerateDuringChatGPTTokenRefresh(t *testing.T) {
	f := newFixture(t)
	f.chatgptReady(t)
	entered := make(chan struct{}, 1)
	f.ChatGPT.OnAccess = func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done() // ネットワーク往復でブロックしている更新。中止で ctx が終わる
		return ctx.Err()
	}
	for name, call := range f.entries(t) {
		if name == "FindInboxZeroCandidates" {
			continue // Inbox Zero は ChatGPT を使わない
		}
		t.Run(name, func(t *testing.T) {
			id := "refresh-" + name
			done := make(chan error, 1)
			go func() { done <- call(id) }()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("トークン更新に入らなかった")
			}
			require.NoError(t, f.ai.CancelGenerate(id))
			select {
			case err := <-done:
				requireCode(t, "CANCELLED", err)
			case <-time.After(10 * time.Second):
				t.Fatal("中止してもトークン更新が中断されず、生成が終わらなかった")
			}
			assert.Zero(t, f.provider.count())
		})
	}
}

// ChatGPT でも、登録前の中止ではトークンの取得（更新）をしない。
func TestCancelGenerateBeforeStartSkipsChatGPTToken(t *testing.T) {
	f := newFixture(t)
	f.chatgptReady(t)
	require.NoError(t, f.ai.CancelGenerate("r1"))
	_, err := f.ai.GenerateReplyDraft(t.Context(), f.sessionID, "r1", "")
	requireCode(t, "CANCELLED", err)
	assert.Zero(t, f.ChatGPT.AccessCalls)
	assert.Zero(t, f.provider.count())
}

// 生成を始めなかった ID の中止の記録は、TTL を過ぎると消える。あとから同じ ID で始めても止めない（メモリに残り続けず、別の実行も止めない）。
// TTL を 0 にすると記録は直ちに期限切れになるので、待ち時間なしで決定的に確かめられる（最初の呼び出しが CANCELLED なら、期限切れが効いていない）。
func TestCancelRecordExpires(t *testing.T) {
	f := newFixture(t, aicore.WithCancelRecordTTL(0))
	f.ready(t)
	require.NoError(t, f.ai.CancelGenerate("stale"))
	_, err := f.ai.GenerateReplyDraft(t.Context(), f.sessionID, "stale", "")
	require.NoError(t, err)
}

// 実行中の中止は記録を残さない（実行が終わったあと、同じ ID の次の実行は止められない）。
func TestCancelRunningLeavesNoRecord(t *testing.T) {
	f := newFixture(t)
	f.ready(t)
	f.provider.blocked, f.provider.entered = true, make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		_, err := f.ai.GenerateReplyDraft(t.Context(), f.sessionID, "r1", "")
		done <- err
	}()
	select {
	case <-f.provider.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("プロバイダの呼び出しに入らなかった")
	}
	require.NoError(t, f.ai.CancelGenerate("r1"))
	select {
	case err := <-done:
		requireCode(t, "CANCELLED", err)
	case <-time.After(10 * time.Second):
		t.Fatal("中止しても生成が終わらなかった")
	}

	f.provider.mu.Lock()
	f.provider.blocked = false
	f.provider.mu.Unlock()
	_, err := f.ai.GenerateReplyDraft(t.Context(), f.sessionID, "r1", "")
	require.NoError(t, err)
}
