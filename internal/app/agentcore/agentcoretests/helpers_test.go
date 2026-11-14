// Package agentcoretests はエージェントモード（agentcore）を、実 SQLite・フェイクの Gmail・フェイクの AI で検証する。
// 配線は本物（app.NewServices。aicore・composecore・agentcore）で、AI プロバイダだけをフェイクにする。
package agentcoretests

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
)

//nolint:gosec // 以下の secret* はログに出ないことを確かめるためのダミー値で、資格情報ではない
const (
	waitTimeout = 10 * time.Second

	// メールの中身（ログに出てはいけない文字列。資格情報ではなくテスト用のダミー値）。
	secretSubject = "極秘の件名ZZ"
	secretFrom    = "Secretname Alice"
	secretBody    = "本文の秘密BB"
	secretReason  = "理由の秘密RR"
	secretAnswer  = "回答の秘密AA"
	secretQuest   = "質問の秘密QQ"
	secretDraft   = "下書きの秘密DD"
)

// fakeAI は ai.Provider のフェイク。判断・本文づくりを数え、関数で応答を差し替える。
type fakeAI struct {
	mu           sync.Mutex
	judges       int
	generates    int
	instructions []string
	judgeFn      func(call int) (ai.ReplyJudgment, error)
	generateFn   func(call int, in ai.ReplyInput) (string, error)
}

func (p *fakeAI) JudgeReply(_ context.Context, _ ai.ReplyJudgeInput) (ai.ReplyJudgment, error) {
	p.mu.Lock()
	p.judges++
	call, fn := p.judges, p.judgeFn
	p.mu.Unlock()
	if fn != nil {
		return fn(call)
	}
	return ai.ReplyJudgment{NeedsReply: true, ReasonKind: ai.ReasonRequest, Reason: "依頼されています", Questions: []string{}}, nil
}

func (p *fakeAI) GenerateReply(_ context.Context, in ai.ReplyInput) (string, error) {
	p.mu.Lock()
	p.generates++
	p.instructions = append(p.instructions, in.Instruction)
	call, fn := p.generates, p.generateFn
	p.mu.Unlock()
	if fn != nil {
		return fn(call, in)
	}
	return secretDraft + "です。\n\n確認して折り返します。", nil
}

func (p *fakeAI) SummarizeThread(context.Context, ai.ThreadInput) (string, error) {
	return "", errors.New("fakeAI: unexpected SummarizeThread")
}

func (p *fakeAI) AskAboutThread(context.Context, ai.ThreadQuestionInput) (string, error) {
	return "", errors.New("fakeAI: unexpected AskAboutThread")
}

func (p *fakeAI) counts() (judges, generates int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.judges, p.generates
}

func (p *fakeAI) lastInstruction() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.instructions) == 0 {
		return ""
	}
	return p.instructions[len(p.instructions)-1]
}

// syncBuffer は並行に書ける bytes.Buffer（ログの捕捉用）。
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// fakeMascot は deps.MascotWindow のフェイク（SetVisible の履歴を持つ）。
type fakeMascot struct {
	mu      sync.Mutex
	history []bool
}

func (m *fakeMascot) SetVisible(v bool) {
	m.mu.Lock()
	m.history = append(m.history, v)
	m.mu.Unlock()
}

func (m *fakeMascot) got() []bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]bool(nil), m.history...)
}

type fixture struct {
	*apptest.Env
	svc    *app.Services
	ai     *fakeAI
	logs   *syncBuffer
	mascot *fakeMascot
	a, b   store.Account
	cancel context.CancelFunc
}

// newFixture は 2 アカウント（それぞれ別の署名）と、受信箱のスレッド t1（a 宛て）・u1（b 宛て）を持つ環境を作る。
// AI は設定・同意まで済ませてあり（ready）、エージェントはオフ・ワーカーは未起動。
func newFixture(t *testing.T) *fixture {
	t.Helper()
	e := apptest.NewEnv(t)
	f := &fixture{Env: e, ai: &fakeAI{}, logs: &syncBuffer{}, mascot: &fakeMascot{}}
	e.Deps.Logger = slog.New(slog.NewTextHandler(f.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e.Deps.Mascot = f.mascot
	e.Deps.AI = func(string, string, string, string) (ai.Provider, error) { return f.ai, nil }

	f.a = e.AddAccount(t, "me@a.example")
	f.b = e.AddAccount(t, "me@b.example")
	_, err := e.Store.SetSignature(t.Context(), f.a.ID, "<p>署名A</p>")
	require.NoError(t, err)
	_, err = e.Store.SetSignature(t.Context(), f.b.ID, "<p>署名B</p>")
	require.NoError(t, err)

	f.putInbound(t, f.a, "t1", "m1")
	f.putInbound(t, f.b, "u1", "u-m1")
	f.svc = app.NewServices(e.Deps)
	return f
}

// putInbound は受信箱に届いた 1 通のスレッドを Gmail と Store へ置く（件名・差出人・本文は secret*）。
func (f *fixture) putInbound(t *testing.T, acct store.Account, threadID, msgID string) {
	t.Helper()
	m := apptest.Msg(msgID, threadID, apptest.Epoch, "INBOX", "UNREAD")
	m.From = model.Address{Name: secretFrom, Email: "alice@x.example"}
	m.Subject = secretSubject
	m.BodyHTML = "<p>" + secretBody + "</p>"
	f.PutThread(t, acct.ID, threadID, m)
}

// ready は AI のプロバイダ・モデル・キー・同意を済ませる。
func (f *fixture) ready(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	_, err := f.svc.AI.SetAIConfig(ctx, "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	_, err = f.svc.AI.SetAIApiKey(ctx, "openai", "sk-secret-test-key")
	require.NoError(t, err)
	_, err = f.svc.AI.RecordAIConsent(ctx, "openai", "https://llm.example/v1")
	require.NoError(t, err)
}

// configure は AI の設定とキーだけを済ませる（同意はまだ）。
func (f *fixture) configure(t *testing.T) {
	t.Helper()
	_, err := f.svc.AI.SetAIConfig(t.Context(), "openai", "gpt-test", "https://llm.example/v1", "")
	require.NoError(t, err)
	_, err = f.svc.AI.SetAIApiKey(t.Context(), "openai", "sk-secret-test-key")
	require.NoError(t, err)
}

// enable は AI を ready にして、エージェントをオンにし、ワーカーを起動する（終了時に停止・完了待ち）。
func (f *fixture) enable(t *testing.T) {
	t.Helper()
	f.ready(t)
	_, err := f.svc.Agent.SetAgentEnabled(t.Context(), true, true)
	require.NoError(t, err)
	f.start(t)
}

func (f *fixture) start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	f.svc.Agent.Start(ctx)
	t.Cleanup(func() {
		cancel()
		f.svc.Agent.Wait()
	})
}

// stop はワーカーを止めて完了を待つ（それまでに積まれた分の処理が済んだことの保証にも使う）。
func (f *fixture) stop() {
	f.cancel()
	f.svc.Agent.Wait()
}

func (f *fixture) newMail(accountID, threadID, messageID string) {
	f.svc.Agent.OnNewMail(context.Background(), accountID, []syncer.NewMailCandidate{{ThreadID: threadID, MessageID: messageID}})
}

// waitItems は一覧の件数が n になるまで待つ（ワーカーが 1 件処理して保存するたびに 1 件増える。上限つきの条件待ち）。
func (f *fixture) waitItems(t *testing.T, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		o, err := f.svc.Agent.ListAgentItems(context.Background())
		return err == nil && len(o.Items) == n
	}, waitTimeout, 2*time.Millisecond, "一覧が %d 件になるのを待てなかった", n)
}

func (f *fixture) items(t *testing.T) dto.AgentOverview {
	t.Helper()
	o, err := f.svc.Agent.ListAgentItems(t.Context())
	require.NoError(t, err)
	return o
}

func (f *fixture) only(t *testing.T) dto.AgentItem {
	t.Helper()
	o := f.items(t)
	require.Len(t, o.Items, 1)
	return o.Items[0]
}

// requireNeverSent は Gmail の送信が 1 回も呼ばれていないことを確かめる。
func (f *fixture) requireNeverSent(t *testing.T) {
	t.Helper()
	assert.Equal(t, 0, f.Gmail.CallCount("Send"), "エージェントは送信しない")
	assert.Empty(t, f.Gmail.Sent)
}

func requireCode(t *testing.T, want string, err error) {
	t.Helper()
	var ae *apperr.Error
	require.ErrorAs(t, err, &ae)
	require.Equal(t, want, ae.Code)
}

func containsAny(s string, needles ...string) string {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return n
		}
	}
	return ""
}
