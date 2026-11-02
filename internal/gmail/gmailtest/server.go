// Package gmailtest は Gmail API と OAuth トークンエンドポイントを兼ねる偽サーバー（テスト用）を提供する。
package gmailtest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// Request は偽サーバーが受けた 1 リクエスト。
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

// Server は Gmail API と OAuth トークンエンドポイントを兼ねる偽サーバー。
type Server struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	reqs     []Request
	handlers map[string]http.HandlerFunc
	// Token はトークンエンドポイントの応答（nil なら固定のアクセストークン at-N を返す）。テストの開始前に設定する。
	Token      http.HandlerFunc
	tokenCalls int
}

func New(t *testing.T) *Server {
	t.Helper()
	f := &Server{t: t, handlers: map[string]http.HandlerFunc{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// Handle は "METHOD /path" に応答するハンドラを登録する。
func (f *Server) Handle(route string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[route] = h
}

// HandleJSON は status と JSON 本文を固定で返すハンドラを登録する。
func (f *Server) HandleJSON(route string, status int, v any) {
	f.Handle(route, func(w http.ResponseWriter, _ *http.Request) { WriteJSON(w, status, v) })
}

func (f *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	rec := Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body}
	if r.URL.Path == "/token" {
		f.tokenCalls++
		n, tok := f.tokenCalls, f.Token
		f.mu.Unlock()
		r.Body = io.NopCloser(bytes.NewReader(body))
		if tok != nil {
			tok(w, r)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"access_token": fmt.Sprintf("at-%d", n), "token_type": "Bearer", "expires_in": 3600})
		return
	}
	f.reqs = append(f.reqs, rec)
	h := f.handlers[r.Method+" "+r.URL.Path]
	f.mu.Unlock()
	if h == nil {
		f.t.Errorf("想定外のリクエスト: %s %s", r.Method, r.URL.Path)
		WriteJSON(w, http.StatusTeapot, map[string]any{"error": map[string]any{"code": 418, "message": "unexpected"}})
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	h(w, r)
}

// Requests は method・path が一致した受信済みリクエストを返す。
func (f *Server) Requests(method, path string) []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Request
	for _, r := range f.reqs {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func (f *Server) TokenRequests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokenCalls
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// APIError は Gmail API 形式のエラー本文。
func APIError(code int, reason, message string) map[string]any {
	return map[string]any{"error": map[string]any{
		"code": code, "message": message,
		"errors": []map[string]any{{"reason": reason, "message": message}},
	}}
}

// URL はサーバーのベース URL（末尾スラッシュなし）。Gmail API は URL+"/"、トークンは URL+"/token"。
func (f *Server) URL() string { return f.srv.URL }

// Close はサーバーを止める（ネットワーク不通のテスト用）。
func (f *Server) Close() { f.srv.Close() }
