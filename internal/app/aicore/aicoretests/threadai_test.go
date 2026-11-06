package aicoretests

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

func (f *aiFixture) ref() dto.ThreadRef { return dto.ThreadRef{AccountID: f.a.ID, ThreadID: threadID} }

// putLongThread は threadID のスレッドに、本文が bodyRunes 文字のメッセージを n 通（古い順）置く。
func (f *aiFixture) putLongThread(t *testing.T, tid string, n, bodyRunes int) dto.ThreadRef {
	t.Helper()
	msgs := make([]gmail.MessageFull, n)
	for i := range msgs {
		m := apptest.Msg(fmt.Sprintf("%s-%d", tid, i), tid, apptest.Epoch.Add(time.Duration(i)*time.Minute), "INBOX")
		m.BodyHTML = ""
		m.BodyText = strings.Repeat("あ", bodyRunes)
		msgs[i] = m
	}
	f.PutThread(t, f.a.ID, tid, msgs...)
	return dto.ThreadRef{AccountID: f.a.ID, ThreadID: tid}
}

// 要約はスレッドの全メッセージ（下書きを除く）を、添付はファイル名とサイズだけで渡す。上限内なら includedCount＝totalCount（F-17）。
func TestSummarizeThreadInput(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	m1 := apptest.Msg("s1", "t3", apptest.Epoch, "INBOX")
	m1.Attachments = []model.AttachmentInfo{{ID: "att1", Filename: "見積.pdf", MimeType: "application/pdf", Size: 2048}}
	m2 := apptest.Msg("s2", "t3", apptest.Epoch.Add(time.Minute), "INBOX")
	draft := apptest.Msg("s3", "t3", apptest.Epoch.Add(2*time.Minute), "DRAFT")
	draft.BodyHTML = "<p>書きかけ</p>"
	f.PutThread(t, f.a.ID, "t3", m1, m2, draft)

	res, err := f.ai.SummarizeThread(t.Context(), dto.ThreadRef{AccountID: f.a.ID, ThreadID: "t3"}, "r1")
	require.NoError(t, err)
	assert.Equal(t, dto.ThreadAIResult{Text: "- 回答", IncludedCount: 2, TotalCount: 2}, res)

	require.Len(t, f.provider.summaries, 1)
	in := f.provider.summaries[0]
	require.Len(t, in.Messages, 2, "下書きは含めない")
	assert.Equal(t, []ai.AttachmentRef{{Filename: "見積.pdf", Size: 2048}}, in.Messages[0].Attachments)
	assert.Empty(t, in.Messages[1].Attachments)
	assert.Equal(t, f.a.Email, in.AccountEmail)
	assert.Empty(t, f.provider.inputs, "返信下書きの入口は呼ばない")
	assert.Equal(t, "openai", f.factory.provider)
}

// 合計 40,000 文字を超えると古いメッセージから外し、includedCount < totalCount で知らせる。
func TestSummarizeThreadTruncatesFromTheOldest(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	ref := f.putLongThread(t, "long5", 5, 15000) // 5 通 × 15,000 文字。新しい 2 通（30,000）まで入る

	res, err := f.ai.SummarizeThread(t.Context(), ref, "r1")
	require.NoError(t, err)
	assert.Equal(t, 2, res.IncludedCount)
	assert.Equal(t, 5, res.TotalCount)
	require.Len(t, f.provider.summaries[0].Messages, 2)

	// 最新の 1 件だけで超えるときは、その本文の先頭を切り詰める。
	ref = f.putLongThread(t, "long1", 1, 50000)
	res, err = f.ai.SummarizeThread(t.Context(), ref, "r2")
	require.NoError(t, err)
	assert.Equal(t, 1, res.IncludedCount)
	assert.Len(t, []rune(f.provider.summaries[1].Messages[0].Text), 40000)
}

func TestSummarizeThreadPreconditions(t *testing.T) {
	f := newAIFixture(t)
	_, err := f.ai.SummarizeThread(t.Context(), f.ref(), "r1")
	requireCode(t, "AI_NOT_CONFIGURED", err)
	_, err = f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(t.Context(), "openai", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.SummarizeThread(t.Context(), f.ref(), "r1")
	requireCode(t, "AI_CONSENT_REQUIRED", err)
	assert.Empty(t, f.provider.summaries, "同意前は送らない")

	f.ready(t)
	_, err = f.ai.SummarizeThread(t.Context(), f.ref(), "")
	requireCode(t, "VALIDATION", err)
	_, err = f.ai.SummarizeThread(t.Context(), dto.ThreadRef{AccountID: f.a.ID, ThreadID: "none"}, "r2")
	requireCode(t, "NOT_FOUND", err)
}

func TestSummarizeThreadSetsEffortAndFailures(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	_, err := f.ai.SetAIConfig(t.Context(), "openai", "gpt-test", "https://llm.example/v1", "high")
	require.NoError(t, err)
	_, err = f.ai.SummarizeThread(t.Context(), f.ref(), "r1")
	require.NoError(t, err)
	assert.Equal(t, "high", f.provider.summaries[0].Effort)

	f.provider.threadFn = func(context.Context) (string, error) { return "", errors.Join(ai.ErrRateLimited) }
	_, err = f.ai.SummarizeThread(t.Context(), f.ref(), "r2")
	requireCode(t, "RATE_LIMITED", err)
	f.provider.threadFn = func(context.Context) (string, error) { return " \n", nil }
	_, err = f.ai.SummarizeThread(t.Context(), f.ref(), "r3")
	requireCode(t, "AI_FAILED", err)
}

func TestThreadAICancelAndTimeout(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	f.provider.entered = make(chan struct{})
	f.provider.threadFn = func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.ai.SummarizeThread(t.Context(), f.ref(), "r-cancel")
		done <- err
	}()
	<-f.provider.entered
	require.NoError(t, f.ai.CancelGenerate("r-cancel"))
	requireCode(t, "CANCELLED", <-done)

	f.ai = aicore.New((*deps.Deps)(f.Deps), f.svc, aicore.WithTimeout(10*time.Millisecond))
	f.provider.entered = nil
	_, err := f.ai.AskAboutThread(t.Context(), f.ref(), "r-timeout", "いつまで？", nil)
	ae := requireError(t, "AI_FAILED", err)
	assert.True(t, ae.Retryable)
}

// 要約の本文・スレッドの内容は、エラー文面・ログ・設定に残さない。
func TestSummaryIsNotLoggedOrStored(t *testing.T) {
	f := newAIFixture(t)
	f.ready(t)
	const sensitive = "社外秘の要約本文"
	f.provider.threadFn = func(context.Context) (string, error) { return sensitive, nil }
	_, err := f.ai.SummarizeThread(t.Context(), f.ref(), "r1")
	require.NoError(t, err)
	assert.NotContains(t, f.logs.String(), sensitive)
	cfg, _, err := f.Store.GetSetting(t.Context(), store.SettingAIConfig)
	require.NoError(t, err)
	assert.NotContains(t, cfg, sensitive)
}
