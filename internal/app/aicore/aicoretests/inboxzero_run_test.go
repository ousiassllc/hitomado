package aicoretests

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// manyRefs は存在しないスレッドの ref を n 件作る。
func (f *izFixture) manyRefs(n int) []dto.ThreadRef {
	refs := make([]dto.ThreadRef, n)
	for i := range refs {
		refs[i] = dto.ThreadRef{AccountID: f.a.ID, ThreadID: fmt.Sprintf("missing-%d", i)}
	}
	return refs
}

// 理由がすべてオフ・101 件・リクエスト ID なしは VALIDATION。重複は除いて数え、順序を保って 1 回だけ判定する。
func TestInboxZeroValidationAndDedupe(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	a, b := f.mail(t, "t-a"), f.mail(t, "t-b")
	f.setReasons(t, dto.InboxZeroReasons{})
	_, err := f.find(a)
	requireCode(t, "VALIDATION", err)
	f.setReasons(t, allReasons)
	_, err = f.ai.FindInboxZeroCandidates(t.Context(), []dto.ThreadRef{a}, "")
	requireCode(t, "VALIDATION", err)
	_, err = f.find(f.manyRefs(101)...)
	requireCode(t, "VALIDATION", err)
	res, err := f.find(f.manyRefs(100)...)
	require.NoError(t, err)
	assert.Equal(t, 100, res.Examined)
	assert.Equal(t, 100, res.Skipped, "存在しないスレッドは skipped")

	f.answers(map[string]ai.ClassifyAnswer{"t-a": pick("ad", 0.9), "t-b": pick("ad", 0.9)})
	n := f.Classifier.Calls()
	res, err = f.find(b, a, b, a)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Examined)
	assert.Equal(t, n+2, f.Classifier.Calls())
	assert.Equal(t, []dto.ThreadRef{b, a}, []dto.ThreadRef{res.Candidates[0].Ref, res.Candidates[1].Ref})
}

// Jev の理由がオンでキーが無ければ AI_NOT_CONFIGURED（返信下書き用の設定は前提にならない）、同意が無ければ AI_CONSENT_REQUIRED。通信しない。
func TestInboxZeroPreconditions(t *testing.T) {
	f := newIZFixture(t)
	ref := f.mail(t, "t-ad")
	_, err := f.find(ref)
	assert.Contains(t, requireError(t, "AI_NOT_CONFIGURED", err).Message, "TypeSafe AI のキー")
	f.ready(t)
	_, err = f.ai.RecordAIConsent(t.Context(), "typesafe", "")
	require.NoError(t, err)
	_, err = f.find(ref)
	requireCode(t, "AI_NOT_CONFIGURED", err)
	_, err = f.ai.SetAIApiKey(t.Context(), "typesafe", testAPIKey)
	require.NoError(t, err)
	_, err = f.find(ref)
	require.NoError(t, err)

	f2 := newIZFixture(t)
	_, err = f2.ai.SetAIApiKey(t.Context(), "typesafe", testAPIKey)
	require.NoError(t, err)
	_, err = f2.find(f2.mail(t, "t-ad"))
	requireCode(t, "AI_CONSENT_REQUIRED", err)
	assert.Zero(t, f.Classifier.Created()-1+f2.Classifier.Created()+f2.Classifier.Calls(), "前提を満たすまで通信しない")
}

// Jev の失敗は、そのスレッドだけ skipped（候補にしない）。ほかは続ける。
func TestInboxZeroSingleFailureIsSkipped(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	refs := []dto.ThreadRef{f.mail(t, "t-good"), f.mail(t, "t-bad"), f.mail(t, "t-rej")}
	f.Classifier.Fn = func(_ context.Context, in ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		switch {
		case strings.Contains(in.State, "subject t-bad"):
			return ai.ClassifyAnswer{}, fmt.Errorf("%w: HTTP 500", ai.ErrFailed)
		case strings.Contains(in.State, "subject t-rej"):
			return ai.ClassifyAnswer{}, fmt.Errorf("%w: HTTP 400", ai.ErrRejected)
		}
		return pick("ad", 0.9), nil
	}
	res, err := f.find(refs...)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Examined)
	assert.Equal(t, 2, res.Skipped)
	assert.Equal(t, map[string]string{"t-good": "ad"}, reasonsOf(res))
}

// 401／403 と 429 は全体を止め、対応するエラーにする。
func TestInboxZeroAuthAndRateLimitAbort(t *testing.T) {
	for jevErr, code := range map[error]string{
		fmt.Errorf("%w: HTTP 401", ai.ErrUnauthorized): "AI_FAILED", fmt.Errorf("%w: HTTP 429", ai.ErrRateLimited): "RATE_LIMITED",
	} {
		f := newIZFixture(t)
		f.jevReady(t)
		refs := make([]dto.ThreadRef, 12)
		for i := range refs {
			refs[i] = f.mail(t, fmt.Sprintf("t%d", i))
		}
		f.Classifier.Fn = func(_ context.Context, in ai.ClassifyInput) (ai.ClassifyAnswer, error) {
			if strings.Contains(in.State, "subject t0\n") {
				return ai.ClassifyAnswer{}, jevErr
			}
			return pick("ad", 0.9), nil
		}
		res, err := f.find(refs...)
		assert.NotContains(t, requireError(t, code, err).Message, testAPIKey)
		assert.Empty(t, res.Candidates)
	}
}

// CancelGenerate は実行中の判定を止めて CANCELLED。呼び出し側の ctx の中止も同じ。同じ ID は使い回せる。
func TestInboxZeroCancel(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	ref := f.mail(t, "t-ad")
	entered, once := make(chan struct{}), sync.Once{}
	f.Classifier.Fn = func(ctx context.Context, _ ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return ai.ClassifyAnswer{}, ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.ai.FindInboxZeroCandidates(t.Context(), []dto.ThreadRef{ref}, "iz-cancel")
		done <- err
	}()
	<-entered
	require.NoError(t, f.ai.CancelGenerate("iz-cancel"))
	requireCode(t, "CANCELLED", <-done)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := f.ai.FindInboxZeroCandidates(ctx, []dto.ThreadRef{ref}, "iz-cancel")
	requireCode(t, "CANCELLED", err)
}

// 同時に走る Jev への要求は 4 件まで。
func TestInboxZeroBoundedConcurrency(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	refs := make([]dto.ThreadRef, 12)
	for i := range refs {
		refs[i] = f.mail(t, fmt.Sprintf("t%d", i))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var inflight, peak atomic.Int32
	gate, once := make(chan struct{}), sync.Once{}
	f.Classifier.Fn = func(ctx context.Context, _ ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		n := inflight.Add(1)
		defer inflight.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		if n == 4 {
			once.Do(func() { close(gate) }) // 4 件が揃ったら全員を通す
		}
		select {
		case <-gate:
		case <-ctx.Done():
		}
		return pick("ad", 0.9), nil
	}
	res, err := f.ai.FindInboxZeroCandidates(ctx, refs, "iz-conc")
	require.NoError(t, err)
	assert.Len(t, res.Candidates, 12)
	assert.EqualValues(t, 4, peak.Load())
}

// 送る内容は差出人・件名・最新の（下書きでない）メッセージの本文の先頭 500 文字だけ。キーも本文もログに出ない。
func TestInboxZeroSentStateAndLogs(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	older := msg("ss1", "t-s", 0, "INBOX")
	older.BodyHTML = "<p>古い本文SECRETOLD</p>"
	latest := msg("ss2", "t-s", 1, "INBOX")
	latest.From, latest.Subject = model.Address{Name: "Shop", Email: "shop@example.com"}, "大セール"
	latest.BodyHTML = "<p>" + strings.Repeat("あ", 600) + "</p>"
	draft := msg("ss3", "t-s", 2, "DRAFT")
	draft.BodyHTML = "<p>書きかけSECRETDRAFT</p>"
	bare := msg("n1", "t-n", 0, "INBOX")
	bare.From, bare.Subject, bare.BodyHTML, bare.BodyText = model.Address{Email: "bare@example.com"}, "短い", "", "  テキスト本文  "
	_, err := f.find(f.thread(t, "t-s", older, latest, draft), f.thread(t, "t-n", bare))
	require.NoError(t, err)

	var states []string
	for _, in := range f.Classifier.Inputs {
		assert.NotEmpty(t, in.Instructions)
		states = append(states, in.State)
	}
	assert.ElementsMatch(t, []string{
		"From: Shop <shop@example.com>\nSubject: 大セール\n\n" + strings.Repeat("あ", 500),
		"From: bare@example.com\nSubject: 短い\n\nテキスト本文",
	}, states)
	for _, secret := range []string{testAPIKey, "SECRETOLD", "SECRETDRAFT", "大セール", "あああ", "bare@example.com"} {
		assert.NotContains(t, f.logs.String(), secret)
	}
}
