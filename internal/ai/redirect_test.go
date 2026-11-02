package ai_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

// リダイレクト先（別ホスト）にはリクエストが届かず、3xx は ErrFailed になる。
func TestRedirectIsNotFollowed(t *testing.T) {
	const leak = "leaked-redirect-body"
	for _, name := range []string{"anthropic", "openai"} {
		for _, status := range []int{301, 302, 307, 308} {
			t.Run(name+"/"+http.StatusText(status), func(t *testing.T) {
				hit := make(chan struct{}, 1)
				target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					hit <- struct{}{}
					_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}],"content":[{"type":"text","text":"ok"}]}`)
				}))
				t.Cleanup(target.Close)
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Location", target.URL+r.URL.Path)
					w.WriteHeader(status)
					_, _ = io.WriteString(w, leak)
				}))
				t.Cleanup(origin.Close)

				_, err := provider(t, name, origin.URL).GenerateReply(context.Background(), input)
				require.ErrorIs(t, err, ai.ErrFailed)
				assert.NotContains(t, err.Error(), leak)
				assert.NotContains(t, err.Error(), apiKey)
				select {
				case <-hit:
					t.Fatal("リダイレクト先にリクエストが届いた")
				default:
				}
			})
		}
	}
}

func TestNewRejectsUnsafeBaseURL(t *testing.T) {
	bad := []string{
		"https://user:pass@llm.example.com/v1",
		"https://user@llm.example.com/v1",
		"http://user:pass@localhost:1234/v1",
		"https://llm.example.com/v1?key=secret",
		"https://llm.example.com/v1?",
		"https://llm.example.com/v1#frag",
		"https://llm.example.com/v1#",
		"ftp://llm.example.com/v1",
		"https://",
	}
	for _, u := range bad {
		_, err := ai.New("openai", "m", u, "k")
		require.ErrorIs(t, err, ai.ErrInvalidBaseURL, u)
		assert.NotContains(t, err.Error(), "secret")
		assert.NotContains(t, err.Error(), "pass")
	}
	for _, u := range []string{"http://localhost:1234/v1", "http://127.0.0.1:11434/v1", "http://[::1]:8080/v1", "https://llm.example.com/v1/"} {
		_, err := ai.New("openai", "m", u, "k")
		require.NoError(t, err, u)
	}
}

func TestBaseURLHTTPOnlyForLoopback(t *testing.T) {
	allowed := []string{
		"http://localhost", "http://localhost:1234/v1", "http://127.0.0.1", "http://127.0.0.1:11434/v1",
		"http://[::1]:8080", "http://[::1]/v1", "https://example.com", "https://192.168.0.5:8443/v1",
	}
	for _, u := range allowed {
		assert.NoError(t, ai.ValidateBaseURL(u), u)
	}
	insecure := []string{
		"http://example.com/v1", "http://192.168.0.5:11434/v1", "http://localhost.evil.com/v1",
		"http://127.0.0.1.evil.com", "http://0.0.0.0:8080", "http://127.1", "http://127.0.0.2",
		"http://[::ffff:127.0.0.1]:8080", "http://localhost./v1", "http://LOCALHOST:8080", "http://127.0.0.1.:8080",
		"http://localhost@evil.com", "http://[::]:8080",
	}
	for _, u := range insecure {
		err := ai.ValidateBaseURL(u)
		require.Error(t, err, u)
		assert.ErrorIs(t, err, ai.ErrInvalidBaseURL, u)
	}
	for _, u := range []string{"http://example.com/v1", "http://localhost.evil.com/v1", "http://0.0.0.0:8080", "http://127.1"} {
		assert.ErrorIs(t, ai.ValidateBaseURL(u), ai.ErrInsecureBaseURL, u)
		_, err := ai.New("openai", "m", u, "k")
		assert.ErrorIs(t, err, ai.ErrInsecureBaseURL, u)
		assert.NotContains(t, err.Error(), "evil")
	}
	assert.NotErrorIs(t, ai.ValidateBaseURL("https://user@x.example"), ai.ErrInsecureBaseURL)
}
