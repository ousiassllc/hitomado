package ai_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// listFor は provider ごとの ListModels 呼び出し。anthropic・chatgpt は baseURL を使うため hc を渡す。
func listFor(provider, base string) ([]ai.ModelInfo, error) {
	return ai.ListModels(context.Background(), provider, base, apiKey, ai.NewHTTPClient())
}

var allProviders = []string{ai.ProviderOpenAI, ai.ProviderAnthropic, ai.ProviderChatGPT}

func TestListModelsOpenAI(t *testing.T) {
	srv, rec := serve(t, 200, `{"object":"list","data":[{"id":"gpt-b"},{"id":""},{"id":"gpt-a"},{"id":"gpt-b"},{"object":"x"}]}`)
	got, err := ai.ListModels(context.Background(), ai.ProviderOpenAI, srv.URL+"/", apiKey, nil)
	require.NoError(t, err)
	assert.Equal(t, []ai.ModelInfo{{ID: "gpt-b"}, {ID: "gpt-a"}}, got)
	assert.Equal(t, http.MethodGet, rec.method)
	assert.Equal(t, "/models", rec.path)
	assert.Equal(t, "Bearer "+apiKey, rec.header.Get("Authorization"))
}

func TestListModelsChatGPT(t *testing.T) {
	srv, rec := serve(t, 200, `{"models":[
		{"slug":"gpt-5","display_name":"GPT-5","visibility":"list"},
		{"slug":"hidden","display_name":"Hidden","visibility":"hide"},
		{"slug":"nov","display_name":"No visibility"},
		{"slug":"","display_name":"Empty","visibility":"list"},
		{"slug":"gpt-4","visibility":"list"},
		{"slug":"gpt-5","display_name":"dup","visibility":"list"}
	]}`)
	got, err := listFor(ai.ProviderChatGPT, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, []ai.ModelInfo{{ID: "gpt-5", DisplayName: "GPT-5"}, {ID: "gpt-4"}}, got)
	assert.Equal(t, http.MethodGet, rec.method)
	assert.Equal(t, "/models", rec.path)
	assert.Equal(t, "Bearer "+apiKey, rec.header.Get("Authorization"))
}

func TestListModelsAnthropicPagination(t *testing.T) {
	type req struct{ query, key, version string }
	var reqs []req
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs = append(reqs, req{r.URL.RawQuery, r.Header.Get("x-api-key"), r.Header.Get("anthropic-version")})
		assert.Equal(t, "/v1/models", r.URL.Path)
		switch r.URL.Query().Get("after_id") {
		case "":
			_, _ = io.WriteString(w, `{"data":[{"id":"claude-1","display_name":"Claude 1"},{"id":"claude-2","display_name":"Claude 2"}],"has_more":true,"last_id":"claude-2"}`)
		case "claude-2":
			_, _ = io.WriteString(w, `{"data":[{"id":"claude-2","display_name":"dup"},{"id":"claude-3"}],"has_more":true,"last_id":"claude/3 x"}`)
		case "claude/3 x":
			_, _ = io.WriteString(w, `{"data":[{"id":"claude-4","display_name":"Claude 4"}],"has_more":false,"last_id":"claude-4"}`)
		default:
			t.Errorf("想定外の after_id: %q", r.URL.Query().Get("after_id"))
		}
	}))
	t.Cleanup(srv.Close)

	got, err := listFor(ai.ProviderAnthropic, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, []ai.ModelInfo{
		{ID: "claude-1", DisplayName: "Claude 1"},
		{ID: "claude-2", DisplayName: "Claude 2"},
		{ID: "claude-3"},
		{ID: "claude-4", DisplayName: "Claude 4"},
	}, got)
	require.Len(t, reqs, 3)
	assert.Equal(t, "limit=1000", reqs[0].query)
	assert.Equal(t, "limit=1000&after_id=claude-2", reqs[1].query)
	assert.Equal(t, "limit=1000&after_id=claude%2F3+x", reqs[2].query)
	for _, r := range reqs {
		assert.Equal(t, apiKey, r.key)
		assert.Equal(t, "2023-06-01", r.version)
	}
}

func TestListModelsAnthropicStopsAtMaxPages(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		id := "m" + strconv.Itoa(int(n))
		_, _ = io.WriteString(w, `{"data":[{"id":"`+id+`"}],"has_more":true,"last_id":"`+id+`"}`)
	}))
	t.Cleanup(srv.Close)

	got, err := listFor(ai.ProviderAnthropic, srv.URL)
	require.NoError(t, err)
	assert.Len(t, got, 5)
	assert.EqualValues(t, 5, calls.Load())
}

func TestListModelsAnthropicHasMoreWithoutLastIDStops(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"data":[{"id":"m1"}],"has_more":true}`)
	}))
	t.Cleanup(srv.Close)

	got, err := listFor(ai.ProviderAnthropic, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, []ai.ModelInfo{{ID: "m1"}}, got)
	assert.EqualValues(t, 1, calls.Load())
}

func TestListModelsEmptyIsNonNilSlice(t *testing.T) {
	for _, name := range allProviders {
		for _, body := range []string{`{}`, `{"data":[],"models":[]}`, `{"data":[{"id":""}],"models":[{"slug":"x","visibility":"hide"}]}`} {
			t.Run(name+"/"+body, func(t *testing.T) {
				srv, _ := serve(t, 200, body)
				got, err := listFor(name, srv.URL)
				require.NoError(t, err)
				assert.NotNil(t, got)
				assert.Empty(t, got)
			})
		}
	}
}

func TestListModelsUnsupported(t *testing.T) {
	for _, name := range allProviders {
		for _, status := range []int{404, 405, 501} {
			t.Run(name+"/"+strconv.Itoa(status), func(t *testing.T) {
				srv := echoServer(t, status, "application/json", `{"error":{"message":{{ECHO}}}}`)
				_, err := listFor(name, srv.URL)
				require.ErrorIs(t, err, ai.ErrModelsUnsupported)
				assert.NotErrorIs(t, err, ai.ErrFailed)
				assertNoLeak(t, err)
			})
		}
	}
}

// 失敗の分類と、機密（キー・サーバー詳細文）がエラーに現れないこと。
func TestListModelsErrorClassificationAndSecretHiding(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"401", 401, `{"error":{"message":{{ECHO}}},"detail":{{ECHO}}}`, ai.ErrUnauthorized},
		{"403", 403, `{"detail":{{ECHO}}}`, ai.ErrUnauthorized},
		{"429", 429, `{"error":{"code":"subscription_sharing_usage_limit_exceeded","message":{{ECHO}}}}`, ai.ErrRateLimited},
		{"500", 500, `{"detail":{{ECHO}}}`, ai.ErrFailed},
		{"503", 503, serverDetail + `{{ECHO}}`, ai.ErrFailed},
		{"不正な JSON", 200, serverDetail + `{{ECHO}}`, ai.ErrFailed},
		{"配列", 200, `[{{ECHO}}]`, ai.ErrFailed},
	}
	for _, name := range allProviders {
		for _, c := range cases {
			t.Run(name+"/"+c.name, func(t *testing.T) {
				srv := echoServer(t, c.status, "application/json", c.body)
				got, err := listFor(name, srv.URL)
				require.ErrorIs(t, err, c.want)
				assert.Nil(t, got)
				assertNoLeak(t, err)
			})
		}
	}
}

func TestListModelsRedirectIsNotFollowed(t *testing.T) {
	for _, name := range allProviders {
		for _, status := range []int{301, 302, 307, 308} {
			t.Run(name+"/"+http.StatusText(status), func(t *testing.T) {
				var hits atomic.Int32
				target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					hits.Add(1)
					_, _ = io.WriteString(w, `{"data":[{"id":"x"}],"models":[{"slug":"x","visibility":"list"}]}`)
				}))
				t.Cleanup(target.Close)
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Location", target.URL+r.URL.Path)
					w.WriteHeader(status)
					_, _ = io.WriteString(w, serverDetail)
				}))
				t.Cleanup(origin.Close)

				_, err := listFor(name, origin.URL)
				require.ErrorIs(t, err, ai.ErrFailed)
				assertNoLeak(t, err)
				assert.Zero(t, hits.Load(), "リダイレクト先にリクエストが届いた")
			})
		}
	}
	// hc が nil のとき（openai）の既定クライアントも追従しない。
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(target.Close)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	t.Cleanup(origin.Close)
	_, err := ai.ListModels(context.Background(), ai.ProviderOpenAI, origin.URL, apiKey, nil)
	require.ErrorIs(t, err, ai.ErrFailed)
	assert.Zero(t, hits.Load())
}

func TestListModelsRejectsUnsafeBaseURLWithoutSending(t *testing.T) {
	var calls atomic.Int32
	hc := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(http.NoBody)}, nil
	})}
	srv, rec := serve(t, 200, `{"data":[{"id":"x"}]}`)

	bad := map[string]error{
		"http://example.com/v1":                              ai.ErrInsecureBaseURL,
		"http://localhost.evil.com/v1":                       ai.ErrInsecureBaseURL,
		"https://user:pass@example.com/v1":                   ai.ErrInvalidBaseURL,
		"https://example.com/v1?key=1":                       ai.ErrInvalidBaseURL,
		"https://example.com/v1#frag":                        ai.ErrInvalidBaseURL,
		"ftp://example.com/v1":                               ai.ErrInvalidBaseURL,
		"https://":                                           ai.ErrInvalidBaseURL,
		"http://u:p@" + srv.Listener.Addr().String() + "/v1": ai.ErrInvalidBaseURL,
		srv.URL + "?k=" + apiKey:                             ai.ErrInvalidBaseURL,
	}
	for u, want := range bad {
		for _, name := range allProviders {
			_, err := ai.ListModels(context.Background(), name, u, apiKey, hc)
			require.ErrorIs(t, err, want, "%s %s", name, u)
			assert.NotContains(t, err.Error(), apiKey)
			assert.NotContains(t, err.Error(), "pass")
		}
		_, err := ai.ListModels(context.Background(), ai.ProviderOpenAI, u, apiKey, nil)
		require.ErrorIs(t, err, want, u)
	}
	assert.Zero(t, calls.Load(), "不正な Base URL で通信が発生した")
	assert.Empty(t, rec.method, "不正な Base URL でサーバーに届いた")
}

func TestListModelsDefaultEndpoints(t *testing.T) {
	cases := map[string]struct{ url, header, want string }{
		ai.ProviderOpenAI:    {"https://api.openai.com/v1/models", "Authorization", "Bearer tok"},
		ai.ProviderChatGPT:   {"https://api.openai.com/v1/models", "Authorization", "Bearer tok"},
		ai.ProviderAnthropic: {"https://api.anthropic.com/v1/models?limit=1000", "x-api-key", "tok"},
	}
	for name, want := range cases {
		var gotURL, gotMethod, gotHeader string
		hc := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotURL, gotMethod, gotHeader = r.URL.String(), r.Method, r.Header.Get(want.header)
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"a"}],"models":[{"slug":"a","visibility":"list"}]}`)),
			}, nil
		})}
		got, err := ai.ListModels(context.Background(), name, "", "tok", hc)
		require.NoError(t, err, name)
		assert.Equal(t, []ai.ModelInfo{{ID: "a"}}, got, name)
		assert.Equal(t, want.url, gotURL, name)
		assert.Equal(t, http.MethodGet, gotMethod, name)
		assert.Equal(t, want.want, gotHeader, name)
	}
}

func TestListModelsRejectsUnknownProvider(t *testing.T) {
	_, err := ai.ListModels(context.Background(), "gemini", "", "k", nil)
	require.Error(t, err)
}

func TestListModelsNetworkAndContextErrors(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close() // 接続拒否

	_, err := ai.ListModels(context.Background(), ai.ProviderOpenAI, base, apiKey, nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ai.ErrFailed)
	assert.NotContains(t, err.Error(), "http://", "URL がエラーに含まれる")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ai.ListModels(ctx, ai.ProviderOpenAI, base, apiKey, nil)
	require.ErrorIs(t, err, context.Canceled)
}
