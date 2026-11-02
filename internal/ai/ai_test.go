package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

const apiKey = "sk-test-key-value" //nolint:gosec // テスト用のダミー値

var input = ai.ReplyInput{
	AccountDisplayName: "山田 太郎",
	SignatureText:      "山田 太郎\nExample 株式会社",
	Messages: []ai.ContextMessage{
		{From: "Hanako <hanako@example.com>", Date: time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC), Text: "明日の打ち合わせは可能ですか？"},
		{From: "Taro <taro@example.com>", Date: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC), Text: "はい、可能です。"},
	},
}

// recorded は偽サーバーが受け取ったリクエスト。
type recorded struct {
	method string
	path   string
	header http.Header
	body   map[string]any
}

func serve(t *testing.T, status int, resp string) (*httptest.Server, *recorded) {
	t.Helper()
	rec := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec.method, rec.path, rec.header = r.Method, r.URL.Path, r.Header.Clone()
		_ = json.Unmarshal(raw, &rec.body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func provider(t *testing.T, name, base string) ai.Provider {
	t.Helper()
	p, err := ai.NewWithClient(name, "model-x", base, apiKey, nil)
	require.NoError(t, err)
	return p
}

func TestNewValidation(t *testing.T) {
	_, err := ai.New("gemini", "m", "", "k")
	require.Error(t, err)
	_, err = ai.New("openai", "", "", "k")
	require.Error(t, err)
	_, err = ai.New("openai", "m", "ftp://example.com", "k")
	require.Error(t, err)
	_, err = ai.New("openai", "m", "not a url", "k")
	require.Error(t, err)

	for _, name := range []string{"anthropic", "openai"} {
		p, err := ai.New(name, "m", "", "k")
		require.NoError(t, err, name)
		require.NotNil(t, p)
	}
	_, err = ai.New("openai", "m", "https://llm.example.com/v1/", "k")
	require.NoError(t, err)
}

func TestAnthropicRequestAndResponse(t *testing.T) {
	srv, rec := serve(t, 200, `{"content":[{"type":"text","text":"お世話になっております。"},{"type":"text","text":"承知しました。\n"}]}`)

	got, err := provider(t, "anthropic", srv.URL).GenerateReply(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "お世話になっております。承知しました。", got)

	assert.Equal(t, http.MethodPost, rec.method)
	assert.Equal(t, "/v1/messages", rec.path)
	assert.Equal(t, apiKey, rec.header.Get("x-api-key"))
	assert.Equal(t, "2023-06-01", rec.header.Get("anthropic-version"))
	assert.Equal(t, "application/json", rec.header.Get("Content-Type"))
	assert.Empty(t, rec.header.Get("Authorization"))

	assert.Equal(t, "model-x", rec.body["model"])
	assert.Greater(t, rec.body["max_tokens"], float64(0))
	system, _ := rec.body["system"].(string)
	assertSystemPrompt(t, system)
	msgs, _ := rec.body["messages"].([]any)
	require.Len(t, msgs, 1)
	m, _ := msgs[0].(map[string]any)
	assert.Equal(t, "user", m["role"])
	assertUserPrompt(t, m["content"].(string))
}

func TestOpenAIRequestAndResponse(t *testing.T) {
	srv, rec := serve(t, 200, `{"choices":[{"message":{"role":"assistant","content":"  了解です。 \n"}}]}`)

	got, err := provider(t, "openai", srv.URL+"/v1/").GenerateReply(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "了解です。", got)

	assert.Equal(t, http.MethodPost, rec.method)
	assert.Equal(t, "/v1/chat/completions", rec.path)
	assert.Equal(t, "Bearer "+apiKey, rec.header.Get("Authorization"))
	assert.Empty(t, rec.header.Get("x-api-key"))
	assert.Equal(t, "model-x", rec.body["model"])
	msgs, _ := rec.body["messages"].([]any)
	require.Len(t, msgs, 2)
	sys, _ := msgs[0].(map[string]any)
	usr, _ := msgs[1].(map[string]any)
	assert.Equal(t, "system", sys["role"])
	assertSystemPrompt(t, sys["content"].(string))
	assert.Equal(t, "user", usr["role"])
	assertUserPrompt(t, usr["content"].(string))
}

func assertSystemPrompt(t *testing.T, s string) {
	t.Helper()
	assert.Contains(t, s, "山田 太郎")
	assert.Contains(t, s, "返信の本文だけ")
	assert.Contains(t, s, "署名・引用")
	assert.Contains(t, s, "件名行")
	assert.Contains(t, s, "言語に合わせ")
	assert.Contains(t, s, "重複して書かない")
	assert.Contains(t, s, "Example 株式会社")
}

func assertUserPrompt(t *testing.T, s string) {
	t.Helper()
	first := strings.Index(s, "明日の打ち合わせは可能ですか？")
	second := strings.Index(s, "はい、可能です。")
	require.True(t, first >= 0 && second > first, "古い順に並ぶこと")
	assert.Contains(t, s, "Hanako <hanako@example.com>")
	assert.Contains(t, s, "2026-11-01 09:30")
	assert.NotContains(t, s, "Example 株式会社", "署名はユーザーメッセージに入れない")
}

func TestSystemPromptWithoutSignatureOrName(t *testing.T) {
	srv, rec := serve(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
	_, err := provider(t, "openai", srv.URL).GenerateReply(context.Background(), ai.ReplyInput{Messages: input.Messages})
	require.NoError(t, err)
	msgs, _ := rec.body["messages"].([]any)
	sys, _ := msgs[0].(map[string]any)
	assert.NotContains(t, sys["content"], "<signature>")
}

func TestErrorClassification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"401", 401, `{"error":"` + apiKey + `"}`, ai.ErrUnauthorized},
		{"403", 403, `{}`, ai.ErrUnauthorized},
		{"429", 429, `{"error":"rate limited ` + apiKey + `"}`, ai.ErrRateLimited},
		{"500", 500, `oops`, ai.ErrFailed},
		{"invalid json", 200, `not json`, ai.ErrFailed},
		{"empty object", 200, `{}`, ai.ErrFailed},
		{"empty text", 200, `{"choices":[{"message":{"content":"  "}}],"content":[]}`, ai.ErrFailed},
	}
	for _, name := range []string{"anthropic", "openai"} {
		for _, c := range cases {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				srv, _ := serve(t, c.status, c.body)
				_, err := provider(t, name, srv.URL).GenerateReply(context.Background(), input)
				require.ErrorIs(t, err, c.want)
				if errors.Is(c.want, ai.ErrUnauthorized) {
					assert.NotErrorIs(t, err, ai.ErrFailed)
				}
				assert.NotContains(t, err.Error(), apiKey)
				assert.NotContains(t, err.Error(), "rate limited sk-")
				assert.NotContains(t, err.Error(), "oops")
			})
		}
	}
}

func TestNetworkErrorIsNetError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close() // 接続拒否

	_, err := provider(t, "openai", base).GenerateReply(context.Background(), input)
	require.Error(t, err)
	var ne net.Error
	require.ErrorAs(t, err, &ne)
	assert.NotErrorIs(t, err, ai.ErrFailed)
	assert.NotErrorIs(t, err, ai.ErrUnauthorized)
}

func TestContextCancelIsNotNetError(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body) // 本文を読み切らないとサーバーは切断を検知しない
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	_, err := provider(t, "anthropic", srv.URL).GenerateReply(ctx, input)
	require.ErrorIs(t, err, context.Canceled)
	var ne net.Error
	assert.False(t, errors.As(err, &ne), "キャンセルが OFFLINE に写像されないこと")
}

func TestDefaultEndpoints(t *testing.T) {
	cases := map[string]string{
		"anthropic": "https://api.anthropic.com/v1/messages",
		"openai":    "https://api.openai.com/v1/chat/completions",
	}
	for name, want := range cases {
		var gotURL string
		hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotURL = r.URL.String()
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"content":[{"type":"text","text":"ok"}],"choices":[{"message":{"content":"ok"}}]}`)),
			}, nil
		})}
		p, err := ai.NewWithClient(name, "m", "", "k", hc)
		require.NoError(t, err)
		got, err := p.GenerateReply(context.Background(), input)
		require.NoError(t, err, name)
		assert.Equal(t, "ok", got)
		assert.Equal(t, want, gotURL, name)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewRejectsUnsafeBaseURL(t *testing.T) {
	bad := []string{
		"https://user:pass@llm.example.com/v1",
		"https://user@llm.example.com/v1",
		"https://llm.example.com/v1?key=secret",
		"https://llm.example.com/v1?",
		"https://llm.example.com/v1#frag",
		"https://llm.example.com/v1#",
	}
	for _, u := range bad {
		_, err := ai.New("openai", "m", u, "k")
		require.Error(t, err, u)
		assert.NotContains(t, err.Error(), "secret")
		assert.NotContains(t, err.Error(), "pass")
	}
	for _, u := range []string{"http://192.168.0.5:11434/v1", "http://localhost:1234/v1", "https://llm.example.com/v1/"} {
		_, err := ai.New("openai", "m", u, "k")
		require.NoError(t, err, u)
	}
}
