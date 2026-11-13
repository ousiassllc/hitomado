package aicoretests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// providerCalls はフェイクのプロバイダが呼ばれた回数（返信・要約・質問の合計）。
func (f *aiFixture) providerCalls() int {
	f.provider.mu.Lock()
	defer f.provider.mu.Unlock()
	return len(f.provider.inputs) + len(f.provider.summaries) + len(f.provider.questions)
}

// 生成の入口（返信下書き・要約・質問・Inbox Zero）。同じ requestID で呼ぶ。
func (f *izFixture) entries(t *testing.T) map[string]func(id string) error {
	t.Helper()
	s := f.open(t, "reply")
	ref := f.mail(t, "t-ad")
	return map[string]func(id string) error{
		"GenerateReplyDraft": func(id string) error {
			_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, id, "")
			return err
		},
		"SummarizeThread": func(id string) error {
			_, err := f.ai.SummarizeThread(t.Context(), f.ref(), id)
			return err
		},
		"AskAboutThread": func(id string) error {
			_, err := f.ai.AskAboutThread(t.Context(), f.ref(), id, "いつまで？", nil)
			return err
		},
		"FindInboxZeroCandidates": func(id string) error {
			_, err := f.ai.FindInboxZeroCandidates(t.Context(), []dto.ThreadRef{ref}, id)
			return err
		},
	}
}

// 生成の登録より前に届いた CancelGenerate も効く。プロバイダ（TypeSafe を含む）へは一切送らず CANCELLED。
// 中止の記録は 1 回で消費され、同じ ID でもう一度呼べば（別の実行として）通常どおり動く。
func TestCancelGenerateBeforeStart(t *testing.T) {
	f := newIZFixture(t)
	f.ready(t)
	f.jevReady(t)
	for name, call := range f.entries(t) {
		t.Run(name, func(t *testing.T) {
			providerBefore, classifierBefore := f.providerCalls(), f.Classifier.Calls()
			require.NoError(t, f.ai.CancelGenerate("early-"+name))
			requireCode(t, "CANCELLED", call("early-"+name))
			assert.Equal(t, providerBefore, f.providerCalls(), "中止済みの実行はプロバイダへ送らない")
			assert.Equal(t, classifierBefore, f.Classifier.Calls(), "中止済みの実行は TypeSafe へ送らない")

			require.NoError(t, call("early-"+name), "記録は消費済みなので、同じ ID の次の実行は止めない")
		})
	}
}

// ChatGPT のトークン更新の最中に CancelGenerate が届くと、更新が中断され CANCELLED。プロバイダへは送らない。
func TestCancelGenerateDuringChatGPTTokenRefresh(t *testing.T) {
	f := newIZFixture(t)
	f.chatgptReady(t)
	entered := make(chan struct{}, 1)
	interrupted := make(chan struct{}, 1)
	f.ChatGPT.OnAccess = func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done() // ネットワーク往復でブロックしている更新。中止で ctx が終わる
		interrupted <- struct{}{}
		return ctx.Err()
	}
	for name, call := range f.entries(t) {
		if name == "FindInboxZeroCandidates" {
			continue // Inbox Zero は ChatGPT を使わない
		}
		t.Run(name, func(t *testing.T) {
			id := "refresh-" + name
			providerBefore := f.providerCalls()
			done := make(chan error, 1)
			go func() { done <- call(id) }()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("トークン更新に入らなかった")
			}
			require.NoError(t, f.ai.CancelGenerate(id))
			select {
			case <-interrupted:
			case <-time.After(10 * time.Second):
				t.Fatal("中止してもトークン更新が中断されなかった")
			}
			select {
			case err := <-done:
				requireCode(t, "CANCELLED", err)
			case <-time.After(10 * time.Second):
				t.Fatal("中止しても生成が終わらなかった")
			}
			assert.Equal(t, providerBefore, f.providerCalls())
		})
	}
}

// ChatGPT でも、登録前の中止ではトークンの取得（更新）をしない。
func TestCancelGenerateBeforeStartSkipsChatGPTToken(t *testing.T) {
	f := newAIFixture(t)
	f.chatgptReady(t)
	s := f.open(t, "reply")
	require.NoError(t, f.ai.CancelGenerate("r1"))
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	requireCode(t, "CANCELLED", err)
	assert.Zero(t, f.ChatGPT.AccessCalls)
	assert.Zero(t, f.providerCalls())
}

// 生成を始めなかった ID の中止の記録は、一定時間で消える。あとから同じ ID で始めても止めない（メモリに残り続けず、別の実行も止めない）。
func TestCancelRecordExpires(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.ai = aicore.New((*deps.Deps)(f.Deps), f.svc, aicore.WithCancelRecordTTL(20*time.Millisecond))
	s := f.open(t, "reply")
	require.NoError(t, f.ai.CancelGenerate("stale"))
	require.Eventually(t, func() bool {
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "stale", "")
		return err == nil
	}, 10*time.Second, 5*time.Millisecond)
}

// 実行中の中止は記録を残さない（実行が終わったあと、同じ ID の次の実行は止められない）。
func TestCancelRunningLeavesNoRecord(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.provider.entered = make(chan struct{})
	f.provider.fn = func(ctx context.Context, _ ai.ReplyInput) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	s := f.open(t, "reply")
	done := make(chan error, 1)
	go func() {
		_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
		done <- err
	}()
	<-f.provider.entered
	require.NoError(t, f.ai.CancelGenerate("r1"))
	requireCode(t, "CANCELLED", <-done)

	f.provider.entered = nil
	f.provider.fn = func(context.Context, ai.ReplyInput) (string, error) { return "ok", nil }
	_, err := f.ai.GenerateReplyDraft(t.Context(), s.SessionID, "r1", "")
	require.NoError(t, err)
}
