package siwc

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	callbackPath = "/auth/callback"

	pageDone   = "ログインの操作が完了しました。このタブを閉じて Hitomado に戻ってください。"
	pageDenied = "ログインが完了しませんでした。このタブを閉じて Hitomado に戻ってください。"
)

// callbackResult はリダイレクトで受け取った結果。
type callbackResult struct {
	code     string
	clientID string // 新規登録のときに発行された client_id
	scope    string
	err      error
}

// callbackServer は 127.0.0.1 の一時ポートで 1 回だけ /auth/callback を受け取る待受。
type callbackServer struct {
	srv    *http.Server
	ln     net.Listener
	state  string
	result chan callbackResult // 容量 1。最初の有効なコールバックだけが入る

	closeOnce sync.Once
}

func startCallbackServer(state string) (*callbackServer, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("%w: 待受を開始できません", ErrFailed)
	}
	c := &callbackServer{ln: ln, state: state, result: make(chan callbackResult, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, c.handle)
	c.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = c.srv.Serve(ln) }()
	return c, nil
}

// redirectURI は認可要求と交換で使う URI。docs: localhost ではなく 127.0.0.1、パスは /auth/callback、ポートだけ可変。
func (c *callbackServer) redirectURI() string { return "http://" + c.ln.Addr().String() + callbackPath }

func (c *callbackServer) close() {
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.srv.Shutdown(ctx); err != nil {
			_ = c.srv.Close()
		}
		_ = c.ln.Close()
	})
}

func (c *callbackServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	// state が一致しないリクエストは無視する（待受は続行。第三者に認可を失敗させない）。
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(c.state)) != 1 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res := callbackResult{clientID: q.Get("client_id"), scope: q.Get("scope")}
	msg := pageDone
	switch errCode := q.Get("error"); {
	case errCode == "access_denied":
		res.err, msg = ErrDenied, pageDenied
	case errCode != "":
		res.err, msg = fmt.Errorf("%w: 認可エラー", ErrFailed), pageDenied
	case q.Get("code") == "":
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	default:
		res.code = q.Get("code")
	}
	writePage(w, msg)
	select {
	case c.result <- res:
	default: // 2 回目以降は無視する
	}
}

func writePage(w http.ResponseWriter, msg string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Connection", "close")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	// 書き込み失敗（クライアントの切断）は認可結果の受け取りに影響しない。
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><title>Hitomado</title>
<style>body{font-family:sans-serif;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;background:#f5f5f5;color:#222}main{background:#fff;padding:2rem 2.5rem;border-radius:8px;box-shadow:0 1px 4px rgba(0,0,0,.2);max-width:28rem}h1{font-size:1.2rem;margin:0 0 .75rem}p{margin:0;line-height:1.6}</style>
</head><body><main><h1>Hitomado</h1><p>%s</p></main></body></html>
`, msg)
}
