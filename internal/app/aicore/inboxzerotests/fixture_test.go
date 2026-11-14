package inboxzerotests

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/aicore"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/composecore/composetest"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

const testAPIKey = "sk-secret-test-key" //nolint:gosec // テスト用のダミー値

var (
	requireCode  = composetest.RequireCode
	requireError = composetest.RequireError
)

var allReasons = dto.InboxZeroReasons{Ad: true, Notification: true, Newsletter: true, Replied: true, Calendar: true}

// izFixture は Inbox Zero のテスト環境（アカウント 1 つ。jevReady で TypeSafe AI のキーと同意を済ませる。
// Inbox Zero の AI は未選択のまま＝互換の既定で Jev を使う）。
type izFixture struct {
	*apptest.Env
	ai   *aicore.Service
	a    store.Account
	logs *bytes.Buffer
}

func newIZFixture(t *testing.T) *izFixture {
	t.Helper()
	e := apptest.NewEnv(t)
	f := &izFixture{Env: e, a: e.AddAccount(t, "me@a.example"), logs: &bytes.Buffer{}}
	e.Deps.Logger = slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f.ai = aicore.New((*deps.Deps)(e.Deps), nil) // 作成セッションは使わない
	return f
}

// remoteEmpty はメッセージが 0 件のスレッド（Gmail 側にだけ置く。ストアには 0 件のスレッドを置けない）。
func (f *izFixture) remoteEmpty(tid string) dto.ThreadRef {
	f.PutRemoteThread(f.a.ID, tid)
	return dto.ThreadRef{AccountID: f.a.ID, ThreadID: tid}
}

func (f *izFixture) jevReady(t *testing.T) {
	t.Helper()
	_, err := f.ai.SetAIApiKey(t.Context(), "typesafe", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "typesafe", "")
	require.NoError(t, err)
}

func (f *izFixture) setReasons(t *testing.T, r dto.InboxZeroReasons) {
	t.Helper()
	_, err := f.ai.SetInboxZeroReasons(t.Context(), r)
	require.NoError(t, err)
}

// msg は Epoch の minute 分後のメッセージ。
func msg(id, tid string, minute int, labels ...string) gmail.MessageFull {
	return apptest.Msg(id, tid, apptest.Epoch.Add(time.Duration(minute)*time.Minute), labels...)
}

// mail は tid の受信箱の 1 通のスレッド（件名 "subject <tid>"、差出人 Sender）を置く。
func (f *izFixture) mail(t *testing.T, tid string, labels ...string) dto.ThreadRef {
	return f.thread(t, tid, msg(tid, tid, 0, append([]string{"INBOX"}, labels...)...))
}

func (f *izFixture) thread(t *testing.T, tid string, msgs ...gmail.MessageFull) dto.ThreadRef {
	t.Helper()
	f.PutThread(t, f.a.ID, tid, msgs...)
	return dto.ThreadRef{AccountID: f.a.ID, ThreadID: tid}
}

// answers は件名に tid を含むスレッドへの Jev の答えを決める（無ければ other 1.0）。
func (f *izFixture) answers(m map[string]ai.ClassifyAnswer) {
	f.Classifier.Fn = func(_ context.Context, in ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		for tid, a := range m {
			if strings.Contains(in.State, "Subject: subject "+tid+"\n") {
				return a, nil
			}
		}
		return ai.ClassifyAnswer{Choice: "other", Probabilities: map[string]float64{"other": 1}}, nil
	}
}

func pick(choice string, p float64) ai.ClassifyAnswer {
	return ai.ClassifyAnswer{Choice: choice, Probabilities: map[string]float64{choice: p, "other": 1 - p}}
}

func (f *izFixture) find(refs ...dto.ThreadRef) (dto.InboxZeroResult, error) {
	return f.ai.FindInboxZeroCandidates(context.Background(), refs, "iz-1")
}

func reasonsOf(res dto.InboxZeroResult) map[string]string {
	m := map[string]string{}
	for _, c := range res.Candidates {
		m[c.Ref.ThreadID] = c.Reason
	}
	return m
}
