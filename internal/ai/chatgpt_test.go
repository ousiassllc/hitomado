package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

const (
	serverDetail = "SERVER-DETAIL-TEXT"
	sseType      = "text/event-stream"
)

// leakStrings はエラーに現れてはならない文字列（キー・プロンプト・サーバーの詳細文・要求本文の断片）。
var leakStrings = []string{
	apiKey, serverDetail,
	"明日の打ち合わせ", "山田 太郎", "Example 株式会社", "メールスレッド",
	`"instructions"`, `"store"`,
}

// assertNoLeak は err の文字列表現（%v／%+v／%#v）に機密が含まれないことを確かめる。
func assertNoLeak(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	for _, s := range leakStrings {
		assert.NotContains(t, err.Error(), s)
		assert.NotContains(t, fmt.Sprintf("%+v", err), s)
		assert.NotContains(t, fmt.Sprintf("%#v", err), s)
	}
}

// echoServer は要求本文・Authorization・サーバー詳細文を {{ECHO}}（JSON 文字列として）に埋め込んで返す偽サーバー。
func echoServer(t *testing.T, status int, contentType, tmpl string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		echo := r.Header.Get("Authorization") + "\n" + r.Header.Get("x-api-key") + "\n" + string(raw) + "\n" + serverDetail
		q, _ := json.Marshal(echo)
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, strings.ReplaceAll(tmpl, "{{ECHO}}", string(q)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func chatGPT(t *testing.T, base string) ai.Provider {
	t.Helper()
	p, err := ai.NewWithClient(ai.ProviderChatGPT, "gpt-x", base, apiKey, nil)
	require.NoError(t, err)
	return p
}

func sseEvent(typ, data string) string { return "event: " + typ + "\ndata: " + data + "\n\n" }

func deltaEvent(s string) string {
	b, _ := json.Marshal(s)
	return sseEvent("response.output_text.delta", `{"type":"response.output_text.delta","delta":`+string(b)+`}`)
}

const (
	createdEvent   = `event: response.created` + "\n" + `data: {"type":"response.created","response":{"id":"r1"}}` + "\n\n"
	completedEvent = `event: response.completed` + "\n" + `data: {"type":"response.completed","response":{"id":"r1","status":"completed"}}` + "\n\n"
)

func TestChatGPTRequestAndStream(t *testing.T) {
	stream := ": keepalive\n\n" + createdEvent +
		deltaEvent("こんにちは") + deltaEvent("、") + deltaEvent("世界\n") + completedEvent
	srv, rec := serve(t, 200, stream)

	got, err := chatGPT(t, srv.URL).GenerateReply(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "こんにちは、世界", got)

	assert.Equal(t, http.MethodPost, rec.method)
	assert.Equal(t, "/responses", rec.path)
	assert.Equal(t, "Bearer "+apiKey, rec.header.Get("Authorization"))
	assert.Equal(t, sseType, rec.header.Get("Accept"))
	assert.Equal(t, "application/json", rec.header.Get("Content-Type"))

	keys := make([]string, 0, len(rec.body))
	for k := range rec.body {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"model", "instructions", "input", "store", "stream"}, keys)
	for _, k := range []string{
		"background", "conversation", "max_output_tokens", "max_tool_calls", "metadata", "moderation", "multi_agent",
		"prompt", "prompt_cache_retention", "safety_identifier", "temperature", "top_logprobs", "top_p", "truncation",
		"user", "previous_response_id",
	} {
		assert.NotContains(t, rec.body, k)
	}
	assert.Equal(t, "gpt-x", rec.body["model"])
	assert.Equal(t, false, rec.body["store"])
	assert.Equal(t, true, rec.body["stream"])
	assertSystemPrompt(t, rec.body["instructions"].(string))

	items, ok := rec.body["input"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	assert.Equal(t, map[string]any{"role": "user", "content": item["content"]}, item)
	assertUserPrompt(t, item["content"].(string))
}

func TestChatGPTStreamCRLFAndNoTrailingBlankLine(t *testing.T) {
	stream := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\r\n\r\n" +
		"data: {\"type\":\"response.completed\"}" // 末尾の空行なし
	srv, _ := serve(t, 200, stream)
	got, err := chatGPT(t, srv.URL).GenerateReply(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "ok", got)
}

func TestChatGPTSuccessOnlyAfterCompleted(t *testing.T) {
	cases := map[string]string{
		"completed なしで終了":  createdEvent + deltaEvent("途中まで"),
		"空のストリーム":          "",
		"completed だが本文が空": createdEvent + completedEvent,
		"incomplete":       createdEvent + deltaEvent("途中まで") + sseEvent("response.incomplete", `{"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}`),
		"不正な data":         "data: not-json\n\n" + completedEvent,
		"error（文字列）":       sseEvent("error", `{"type":"error","error":"`+serverDetail+`"}`),
		"failed（コードなし）":    sseEvent("response.failed", `{"type":"response.failed","response":{"error":{"message":"`+serverDetail+`"}}}`),
		"failed（未知コード）":    sseEvent("response.failed", `{"type":"response.failed","response":{"error":{"code":"server_error","message":"`+serverDetail+`"}}}`),
	}
	for name, stream := range cases {
		t.Run(name, func(t *testing.T) {
			srv, _ := serve(t, 200, stream)
			_, err := chatGPT(t, srv.URL).GenerateReply(context.Background(), input)
			require.ErrorIs(t, err, ai.ErrFailed)
			assertNoLeak(t, err)
		})
	}
}

func TestChatGPTStreamAbortedMidway(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", sseType)
		_, _ = io.WriteString(w, createdEvent+deltaEvent("途中"))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler) // チャンク転送の途中で接続を切る
	}))
	t.Cleanup(srv.Close)

	got, err := chatGPT(t, srv.URL).GenerateReply(context.Background(), input)
	require.ErrorIs(t, err, ai.ErrFailed)
	assert.Empty(t, got)
	assertNoLeak(t, err)
}

func TestChatGPTStreamLineTooLong(t *testing.T) {
	huge := `{"type":"response.output_text.delta","delta":"` + strings.Repeat("a", 2<<20) + `"}`
	srv, _ := serve(t, 200, "data: "+huge+"\n\n"+completedEvent)
	_, err := chatGPT(t, srv.URL).GenerateReply(context.Background(), input)
	require.ErrorIs(t, err, ai.ErrFailed)
}

func TestChatGPTContextCancelDuringStream(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", sseType)
		_, _ = io.WriteString(w, createdEvent)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	_, err := chatGPT(t, srv.URL).GenerateReply(ctx, input)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ai.ErrFailed)
}

// 失敗経路の分類と、機密（キー・プロンプト・サーバー詳細文）がエラーに現れないこと。
func TestChatGPTErrorClassificationAndSecretHiding(t *testing.T) {
	const (
		usage    = "subscription_sharing_usage_limit_exceeded"
		notElig  = "subscription_sharing_user_not_eligible"
		invalid  = "subscription_sharing_invalid_user"
		echoBody = `{"error":{"code":"%s","message":{{ECHO}}},"detail":{{ECHO}}}`
	)
	httpBody := func(code string) string { return fmt.Sprintf(echoBody, code) }
	detailOnly := `{"detail":{{ECHO}}}`
	failed := func(code string) string {
		return createdEvent + deltaEvent(serverDetail) +
			sseEvent("response.failed", `{"type":"response.failed","response":{"error":{"code":"`+code+`","message":{{ECHO}}}}}`)
	}
	errEvent := func(code string) string {
		return sseEvent("error", `{"type":"error","code":"`+code+`","message":{{ECHO}}}`)
	}
	nestedErrEvent := sseEvent("error", `{"type":"error","error":{"code":"`+usage+`","message":{{ECHO}}}}`)

	cases := []struct {
		name        string
		status      int
		contentType string
		body        string
		want        error
		notWant     []error
	}{
		{"401", 401, "application/json", httpBody("invalid_api_key"), ai.ErrUnauthorized, []error{ai.ErrFailed}},
		{"401 detail のみ", 401, "application/json", detailOnly, ai.ErrUnauthorized, nil},
		{"401 本文が JSON でない", 401, "text/plain", serverDetail + "{{ECHO}}", ai.ErrUnauthorized, nil},
		{"401 invalid_user", 401, "application/json", httpBody(invalid), ai.ErrUnauthorized, nil},
		{"403 invalid_user", 403, "application/json", httpBody(invalid), ai.ErrUnauthorized, []error{ai.ErrNotEligible}},
		{"403 not_eligible", 403, "application/json", httpBody(notElig), ai.ErrNotEligible, []error{ai.ErrUnauthorized, ai.ErrFailed}},
		{"403 その他", 403, "application/json", httpBody("forbidden"), ai.ErrFailed, []error{ai.ErrNotEligible, ai.ErrUnauthorized}},
		{"403 detail のみ", 403, "application/json", detailOnly, ai.ErrFailed, []error{ai.ErrNotEligible, ai.ErrUnauthorized}},
		{"429 usage limit", 429, "application/json", httpBody(usage), ai.ErrUsageLimit, []error{ai.ErrRateLimited}},
		{"429 その他", 429, "application/json", httpBody("rate_limit_exceeded"), ai.ErrRateLimited, []error{ai.ErrUsageLimit}},
		{"429 detail のみ", 429, "application/json", detailOnly, ai.ErrRateLimited, []error{ai.ErrUsageLimit}},
		{"429 コードが不正形式", 429, "application/json", httpBody("Usage Limit!"), ai.ErrRateLimited, []error{ai.ErrUsageLimit}},
		{"500", 500, "application/json", httpBody("server_error"), ai.ErrFailed, nil},
		{"503", 503, "text/plain", detailOnly, ai.ErrFailed, nil},
		{"418", 418, "text/plain", serverDetail + "{{ECHO}}", ai.ErrFailed, nil},
		{"SSE failed usage limit", 200, sseType, failed(usage), ai.ErrUsageLimit, []error{ai.ErrFailed}},
		{"SSE failed not_eligible", 200, sseType, failed(notElig), ai.ErrNotEligible, nil},
		{"SSE failed invalid_user", 200, sseType, failed(invalid), ai.ErrUnauthorized, nil},
		{"SSE failed その他", 200, sseType, failed("server_error"), ai.ErrFailed, []error{ai.ErrUsageLimit}},
		{"SSE error usage limit", 200, sseType, errEvent(usage), ai.ErrUsageLimit, nil},
		{"SSE error 入れ子の code", 200, sseType, nestedErrEvent, ai.ErrUsageLimit, nil},
		{"SSE error その他", 200, sseType, errEvent("boom"), ai.ErrFailed, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := echoServer(t, c.status, c.contentType, c.body)
			got, err := chatGPT(t, srv.URL).GenerateReply(context.Background(), input)
			require.ErrorIs(t, err, c.want)
			for _, n := range c.notWant {
				assert.NotErrorIs(t, err, n)
			}
			assert.Empty(t, got)
			assertNoLeak(t, err)
		})
	}
}

// 利用上限・対象外のエラーに載るのは固定の機械コードだけ。
func TestChatGPTErrorCarriesOnlyFixedCode(t *testing.T) {
	srv := echoServer(t, 429, "application/json", `{"error":{"code":"subscription_sharing_usage_limit_exceeded","message":{{ECHO}}}}`)
	_, err := chatGPT(t, srv.URL).GenerateReply(context.Background(), input)
	require.ErrorIs(t, err, ai.ErrUsageLimit)
	assert.Contains(t, err.Error(), "subscription_sharing_usage_limit_exceeded")

	srv = echoServer(t, 403, "application/json", `{"error":{"code":"subscription_sharing_user_not_eligible"}}`)
	_, err = chatGPT(t, srv.URL).GenerateReply(context.Background(), input)
	require.ErrorIs(t, err, ai.ErrNotEligible)
	assert.Contains(t, err.Error(), "subscription_sharing_user_not_eligible")
}

func TestChatGPTRedirectIsNotFollowed(t *testing.T) {
	for _, status := range []int{301, 302, 307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var hits atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.Header().Set("Content-Type", sseType)
				_, _ = io.WriteString(w, deltaEvent("ok")+completedEvent)
			}))
			t.Cleanup(target.Close)
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Location", target.URL+r.URL.Path)
				w.WriteHeader(status)
				_, _ = io.WriteString(w, serverDetail)
			}))
			t.Cleanup(origin.Close)

			p, err := ai.NewWithClient(ai.ProviderChatGPT, "gpt-x", origin.URL, apiKey, ai.NewHTTPClient())
			require.NoError(t, err)
			_, err = p.GenerateReply(context.Background(), input)
			require.ErrorIs(t, err, ai.ErrFailed)
			assertNoLeak(t, err)
			assert.Zero(t, hits.Load(), "リダイレクト先にリクエストが届いた")
		})
	}
}

func TestChatGPTDefaultEndpointAndNewIgnoresBaseURL(t *testing.T) {
	var gotURL, gotAuth string
	hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotURL, gotAuth = r.URL.String(), r.Header.Get("Authorization")
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": {sseType}},
			Body:       io.NopCloser(strings.NewReader(deltaEvent("ok") + completedEvent)),
		}, nil
	})}
	p, err := ai.NewWithClient(ai.ProviderChatGPT, "m", "", "tok", hc)
	require.NoError(t, err)
	got, err := p.GenerateReply(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "ok", got)
	assert.Equal(t, "https://api.openai.com/v1/responses", gotURL)
	assert.Equal(t, "Bearer tok", gotAuth)

	// New は chatgpt の baseURL を無視する（不正な値でもエラーにならない）。
	_, err = ai.New(ai.ProviderChatGPT, "m", "ftp://bad", "tok")
	require.NoError(t, err)
	_, err = ai.NewWithClient(ai.ProviderChatGPT, "m", "ftp://bad", "tok", hc)
	require.ErrorIs(t, err, ai.ErrInvalidBaseURL)
	_, err = ai.New(ai.ProviderChatGPT, "", "", "tok")
	require.Error(t, err)
}
