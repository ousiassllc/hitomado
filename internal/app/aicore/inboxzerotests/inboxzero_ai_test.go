package inboxzerotests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
)

// selectAI は「機能ごとに使う AI」の Inbox Zero に provider・model を選ぶ。
func (f *izFixture) selectAI(t *testing.T, provider, model string) {
	t.Helper()
	_, err := f.ai.SetAIFeature(t.Context(), dto.AIFeatureInboxZero, provider, model, "")
	require.NoError(t, err)
}

// openAIServer は Chat Completions の偽サーバー。受け取った要求（本文）を記録し、answer(件名の行を含む本文) の JSON を assistant の応答として返す。
type openAIServer struct {
	*httptest.Server
	mu     sync.Mutex
	auths  []string
	bodies []map[string]any
}

func newOpenAIServer(t *testing.T, answer func(user string) string) *openAIServer {
	t.Helper()
	s := &openAIServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		msgs, _ := body["messages"].([]any)
		user := ""
		if len(msgs) > 0 {
			user, _ = msgs[len(msgs)-1].(map[string]any)["content"].(string)
		}
		s.mu.Lock()
		s.auths = append(s.auths, r.Header.Get("Authorization"))
		s.bodies = append(s.bodies, body)
		s.mu.Unlock()
		content, _ := json.Marshal(answer(user))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%s}}]}`, content)
	}))
	t.Cleanup(s.Close)
	return s
}

// OpenAI だけをつないだ状態（Jev は無い）で、Inbox Zero に選んだ OpenAI 互換・モデルで判定が動く。送る内容は Jev と同じ（差出人・件名・本文の先頭 500 文字）で、
// 確信度が下限（0.8）に満たないものは候補にしない。キー・本文はログに出ない。
func TestInboxZeroRunsOnSelectedOpenAI(t *testing.T) {
	f := newIZFixture(t)
	f.Deps.Classifier = ai.NewClassifierFor // 実の分類器（偽サーバーへ送る）
	srv := newOpenAIServer(t, func(user string) string {
		switch {
		case strings.Contains(user, "Subject: subject t-ad\n"):
			return `{"choice":"ad","confidence":0.95}`
		case strings.Contains(user, "Subject: subject t-low\n"):
			return `{"choice":"ad","confidence":0.6}`
		case strings.Contains(user, "Subject: subject t-note\n"):
			return `{"choice":"notification","confidence":0.9}`
		}
		return `{"choice":"other","confidence":1}`
	})
	ctx := t.Context()
	_, err := f.ai.SetAIBaseURL(ctx, srv.URL)
	require.NoError(t, err)
	_, err = f.ai.SetAIApiKey(ctx, "openai", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(ctx, "openai", srv.URL)
	require.NoError(t, err)
	f.selectAI(t, "openai", "gpt-iz")

	long := msg("long", "t-ad", 0, "INBOX")
	long.BodyHTML = "<p>" + strings.Repeat("あ", 600) + "</p>"
	refs := []dto.ThreadRef{f.thread(t, "t-ad", long), f.mail(t, "t-low"), f.mail(t, "t-note"), f.mail(t, "t-plain")}
	res, err := f.find(refs...)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"t-ad": "ad", "t-note": "notification"}, reasonsOf(res))
	assert.Equal(t, 4, res.Examined)
	assert.Zero(t, res.Skipped)
	assert.Zero(t, f.Classifier.Created()+f.Classifier.Calls(), "Jev の分類器は使わない")

	require.Len(t, srv.bodies, 4)
	for _, a := range srv.auths {
		assert.Equal(t, "Bearer "+testAPIKey, a)
	}
	for _, b := range srv.bodies {
		assert.Equal(t, "gpt-iz", b["model"], "選んだモデルで判定する")
		msgs := b["messages"].([]any)
		sys := msgs[0].(map[string]any)["content"].(string)
		for _, name := range []string{"ad", "notification", "newsletter", "other"} {
			assert.Contains(t, sys, "- "+name+": ")
		}
	}
	var sent string
	for _, b := range srv.bodies {
		if u := b["messages"].([]any)[1].(map[string]any)["content"].(string); strings.Contains(u, "t-ad") {
			sent = u
		}
	}
	assert.Equal(t, "<email>\nFrom: Sender <sender@example.com>\nSubject: subject t-ad\n\n"+strings.Repeat("あ", 500)+"\n</email>", sent)
	assert.NotContains(t, f.logs.String(), testAPIKey)
	assert.NotContains(t, f.logs.String(), "あああ")
}

// 選んだ AI・モデル・送信先・キーが、判定の分類器の工場へそのまま渡る（OpenAI 互換・Anthropic・ChatGPT プラン・Jev）。Jev はモデルを持たない。
func TestInboxZeroUsesSelectedProviderAndModel(t *testing.T) {
	ctx := t.Context()
	for _, tc := range []struct {
		provider, model string
		want            apptest.ClassifierMade
		key             string
		setup           func(t *testing.T, f *izFixture)
	}{
		{"openai", "gpt-x", apptest.ClassifierMade{Provider: "openai", Model: "gpt-x", BaseURL: "https://llm.example/v1"}, testAPIKey, func(t *testing.T, f *izFixture) {
			_, err := f.ai.SetAIBaseURL(ctx, "https://llm.example/v1")
			require.NoError(t, err)
			_, err = f.ai.SetAIApiKey(ctx, "openai", testAPIKey)
			require.NoError(t, err)
			_, err = f.ai.RecordAIConsent(ctx, "openai", "https://llm.example/v1")
			require.NoError(t, err)
		}},
		{"anthropic", "claude-x", apptest.ClassifierMade{Provider: "anthropic", Model: "claude-x"}, testAPIKey, func(t *testing.T, f *izFixture) {
			_, err := f.ai.SetAIApiKey(ctx, "anthropic", testAPIKey)
			require.NoError(t, err)
			_, err = f.ai.RecordAIConsent(ctx, "anthropic", "")
			require.NoError(t, err)
		}},
		{"chatgpt", "gpt-plan", apptest.ClassifierMade{Provider: "chatgpt", Model: "gpt-plan"}, "chatgpt-access-token", func(t *testing.T, f *izFixture) {
			_, err := f.ai.SignInChatGPT(ctx, "")
			require.NoError(t, err)
			_, err = f.ai.RecordAIConsent(ctx, "chatgpt", "")
			require.NoError(t, err)
		}},
		{"typesafe", "", apptest.ClassifierMade{Provider: "typesafe"}, testAPIKey, func(t *testing.T, f *izFixture) {
			f.jevReady(t)
		}},
	} {
		f := newIZFixture(t)
		tc.setup(t, f)
		f.selectAI(t, tc.provider, tc.model)
		f.setReasons(t, dto.InboxZeroReasons{Ad: true})
		_, err := f.find(f.mail(t, "t-ad"))
		require.NoError(t, err, tc.provider)
		assert.Equal(t, []apptest.ClassifierMade{tc.want}, f.Classifier.Made, tc.provider)
		assert.Equal(t, []string{tc.key}, f.Classifier.Keys, tc.provider)
		assert.NotContains(t, f.logs.String(), tc.key, tc.provider)
	}
}

// 使える AI が無いときは、今までどおり AI_NOT_CONFIGURED（キー・ログイン無し）／AI_CONSENT_REQUIRED（同意前）で、通信しない。
// 選んだ AI がつながっていなければ、ほかの AI（つないである Jev・OpenAI）へ切り替えない。
func TestInboxZeroStopsWhenSelectedAIIsUnusable(t *testing.T) {
	ctx := t.Context()
	connect := func(t *testing.T, f *izFixture, provider string, consent bool) {
		t.Helper()
		_, err := f.ai.SetAIApiKey(ctx, provider, testAPIKey)
		require.NoError(t, err)
		if consent {
			_, err = f.ai.RecordAIConsent(ctx, provider, "")
			require.NoError(t, err)
		}
	}

	// 未選択で何もつないでいない → 選ぶ場所を案内する。
	f := newIZFixture(t)
	_, err := f.find(f.mail(t, "t-ad"))
	assert.Contains(t, requireError(t, "AI_NOT_CONFIGURED", err).Message, "機能ごとに使う AI")

	// OpenAI を選んだがキーが無い。Jev はつないであっても、Jev へは切り替えない。
	f = newIZFixture(t)
	f.jevReady(t)
	f.selectAI(t, "openai", "gpt-x")
	_, err = f.find(f.mail(t, "t-ad"))
	assert.Contains(t, requireError(t, "AI_NOT_CONFIGURED", err).Message, "API キー")

	// ChatGPT プランを選んだがログインしていない。
	f = newIZFixture(t)
	f.jevReady(t)
	f.selectAI(t, "chatgpt", "gpt-plan")
	_, err = f.find(f.mail(t, "t-ad"))
	requireCode(t, "AI_NOT_CONFIGURED", err)

	// キーはあるが同意が無い（Jev の同意だけでは足りない。同意はそのプロバイダ・送信先ごと）。
	f = newIZFixture(t)
	f.jevReady(t)
	connect(t, f, "openai", false)
	f.selectAI(t, "openai", "gpt-x")
	_, err = f.find(f.mail(t, "t-ad"))
	requireCode(t, "AI_CONSENT_REQUIRED", err)

	// モデルを選んでいない OpenAI（手で書き換えられた保存値）。
	f = newIZFixture(t)
	connect(t, f, "openai", true)
	require.NoError(t, f.Store.SetSetting(ctx, "ai.features", `{"inboxZero":{"provider":"openai"}}`))
	_, err = f.find(f.mail(t, "t-ad"))
	assert.Contains(t, requireError(t, "AI_NOT_CONFIGURED", err).Message, "モデル")

	assert.Zero(t, f.Classifier.Created()+f.Classifier.Calls(), "前提を満たすまで通信しない")
}

// 未選択のまま既存の #314 の利用者（Jev のキーと同意がある）は、今までどおり Jev で判定する。選んだあとは、選んだ AI が優先される。
func TestInboxZeroUnselectedFallsBackToJev(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.setReasons(t, dto.InboxZeroReasons{Ad: true})
	f.answers(map[string]ai.ClassifyAnswer{"t-ad": pick("ad", 0.9)})
	res, err := f.find(f.mail(t, "t-ad"))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"t-ad": "ad"}, reasonsOf(res))
	assert.Equal(t, []apptest.ClassifierMade{{Provider: "typesafe"}}, f.Classifier.Made)

	_, err = f.ai.SetAIApiKey(t.Context(), "anthropic", testAPIKey)
	require.NoError(t, err)
	_, err = f.ai.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)
	f.selectAI(t, "anthropic", "claude-x")
	_, err = f.find(f.mail(t, "t-ad"))
	require.NoError(t, err)
	assert.Equal(t, "anthropic", f.Classifier.Made[len(f.Classifier.Made)-1].Provider)
}

// 選んだ AI が分類器を作れないとき（不正な Base URL など）は、通信せずに止まる。
func TestInboxZeroClassifierInitFailure(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.Deps.Classifier = func(string, string, string, string) (ai.Classifier, error) { return nil, ai.ErrInvalidBaseURL }
	_, err := f.find(f.mail(t, "t-ad"))
	requireCode(t, "VALIDATION", err)
}

// ChatGPT の利用枠の上限・対象外はどのスレッドでも同じなので全体を止め、プロバイダの失敗（500 相当）は該当スレッドだけ skipped にする。
func TestInboxZeroUsageLimitAborts(t *testing.T) {
	f := newIZFixture(t)
	f.jevReady(t)
	f.Classifier.Fn = func(context.Context, ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		return ai.ClassifyAnswer{}, fmt.Errorf("%w (HTTP 429)", ai.ErrUsageLimit)
	}
	res, err := f.find(f.mail(t, "t-a"), f.mail(t, "t-b"))
	assert.Empty(t, res.Candidates)
	ae := requireError(t, "AI_FAILED", err)
	assert.Equal(t, apperr.ReasonUsageLimit, ae.Details[apperr.DetailReason])
	assert.NotContains(t, ae.Message, testAPIKey)
}
