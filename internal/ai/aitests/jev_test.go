package aitests

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/ai"
)

const jevOKResponse = `{"model":"jev-1.13.0","answers":{"category":{"type":"choice","choice":"ad","confidence":0.9,"probabilities":{"ad":0.9,"notification":0.05,"newsletter":0.03,"other":0.02}}},"usage":{"input_tokens":10,"output_tokens":2}}`

var jevInput = ai.ClassifyInput{
	State:        "From: Shop <shop@example.com>\nSubject: セール\n\n本文",
	Instructions: "種類を選んでください。",
	Criteria:     []ai.Criterion{{Name: "ad", Description: "広告"}, {Name: "other", Description: "その他"}},
}

// serveRaw は serve と同じだが、リクエスト本文を生のまま記録する。
func serveRaw(t *testing.T, status int, resp string) (*httptest.Server, *recorded, *string) {
	t.Helper()
	rec, raw := &recorded{}, new(string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*raw = string(b)
		rec.method, rec.path, rec.header = r.Method, r.URL.Path, r.Header.Clone()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return srv, rec, raw
}

func classifier(t *testing.T, base string) ai.Classifier {
	t.Helper()
	c, err := ai.NewClassifierWithClient(base, apiKey, nil)
	require.NoError(t, err)
	return c
}

// 要求は POST /v1/systemone。Bearer・モデル・choice 質問（選択肢は渡した順）で、state は渡した文字列そのまま。
func TestJevRequestShape(t *testing.T) {
	srv, rec, raw := serveRaw(t, 200, jevOKResponse)
	ans, err := classifier(t, srv.URL).Classify(t.Context(), jevInput)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, rec.method)
	assert.Equal(t, "/v1/systemone", rec.path)
	assert.Equal(t, "Bearer "+apiKey, rec.header.Get("Authorization"))
	assert.Equal(t, "application/json", rec.header.Get("Content-Type"))
	assert.JSONEq(t, `{
		"state": "From: Shop <shop@example.com>\nSubject: セール\n\n本文",
		"model": "jev-latest",
		"questions": {"category": {"type": "choice", "instructions": "種類を選んでください。", "criteria": {"ad": "広告", "other": "その他"}}}
	}`, *raw)
	assert.Contains(t, *raw, `"criteria":{"ad":"広告","other":"その他"}`, "選択肢は渡した順のまま")

	assert.Equal(t, "ad", ans.Choice)
	assert.InDelta(t, 0.9, ans.Probabilities["ad"], 1e-9)
	assert.Len(t, ans.Probabilities, 4)
}

// 401／403 → ErrUnauthorized、429 → ErrRateLimited、それ以外 → ErrFailed。キーも応答本文もエラーに含めない。
func TestJevErrorMapping(t *testing.T) {
	const body = `{"error":"secret-body-detail"}`
	for _, tc := range []struct {
		status int
		want   error
	}{
		{401, ai.ErrUnauthorized}, {403, ai.ErrUnauthorized}, {429, ai.ErrRateLimited}, {500, ai.ErrFailed}, {400, ai.ErrFailed},
	} {
		srv, _ := serve(t, tc.status, body)
		_, err := classifier(t, srv.URL).Classify(t.Context(), jevInput)
		require.ErrorIs(t, err, tc.want, tc.status)
		assert.NotContains(t, err.Error(), apiKey)
		assert.NotContains(t, err.Error(), "secret-body-detail")
		assert.NotContains(t, err.Error(), jevInput.State)
		assert.NotContains(t, err.Error(), srv.URL, "URL は含めない")
	}
}

// 不正な応答（JSON でない・答えが無い・選んだ選択肢の確率が無い）は ErrFailed。
func TestJevMalformedResponses(t *testing.T) {
	for _, resp := range []string{
		`not json`,
		`{}`,
		`{"answers":{"other-id":{"choice":"ad","probabilities":{"ad":1}}}}`,
		`{"answers":{"category":{"choice":"","probabilities":{}}}}`,
		`{"answers":{"category":{"choice":"ad","probabilities":{"other":1}}}}`,
	} {
		srv, _ := serve(t, 200, resp)
		_, err := classifier(t, srv.URL).Classify(t.Context(), jevInput)
		require.ErrorIs(t, err, ai.ErrFailed, resp)
		assert.NotContains(t, err.Error(), "other-id")
	}
}

// ctx の中止はそのまま ctx のエラーで返る（ErrUnauthorized などにはならない）。
func TestJevContextCancel(t *testing.T) {
	srv, _ := serve(t, 200, jevOKResponse)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := classifier(t, srv.URL).Classify(ctx, jevInput)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

// 3xx は追従しない（本文を別の送信先へ再送しない）。
func TestJevDoesNotFollowRedirects(t *testing.T) {
	other, rec := serve(t, 200, jevOKResponse)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	_, err := classifier(t, srv.URL).Classify(t.Context(), jevInput)
	require.ErrorIs(t, err, ai.ErrFailed)
	assert.Empty(t, rec.method, "リダイレクト先には届かない")
}

// 既定の Base URL は TypeSafe AI。不正な Base URL は作れない。
func TestNewClassifierBaseURL(t *testing.T) {
	assert.NotNil(t, ai.NewClassifier(apiKey))
	_, err := ai.NewClassifierWithClient("ftp://x.example", apiKey, nil)
	require.ErrorIs(t, err, ai.ErrInvalidBaseURL)
}
