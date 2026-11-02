package oauth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"html"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	callbackPath = "/callback"

	pageDone   = "認可の操作が完了しました。このタブを閉じて Hitomado に戻ってください。"
	pageDenied = "認可が完了しませんでした。このタブを閉じて Hitomado に戻ってください。"
)

// callbackResult はリダイレクトで受け取った結果。code か err のどちらかが入る。
type callbackResult struct {
	code string
	err  error
}

// callbackServer は 127.0.0.1 の一時ポートで 1 回だけリダイレクトを受け取る待受。
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
		return nil, fmt.Errorf("oauth: 待受を開始できませんでした: %w", err)
	}
	c := &callbackServer{ln: ln, state: state, result: make(chan callbackResult, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, c.handle)
	c.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		// Serve は close 後に ErrServerClosed を返す。それ以外の失敗は認可がタイムアウトで終わるだけなので記録しない。
		_ = c.srv.Serve(ln)
	}()
	return c, nil
}

func (c *callbackServer) redirectURI() string {
	return "http://" + c.ln.Addr().String() + callbackPath
}

// close は待受を閉じる（冪等）。進行中のレスポンス送信は短時間だけ待つ。
func (c *callbackServer) close() {
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.srv.Shutdown(ctx); err != nil {
			_ = c.srv.Close()
		}
		// Serve がまだ起動前だと Shutdown はリスナーを閉じないため、ここで確実に閉じる（二重 Close は無害）。
		_ = c.ln.Close()
	})
}

func (c *callbackServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	// state が一致しないリクエストは無視する（待受は続行。攻撃者に認可を失敗させない）。
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(c.state)) != 1 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var (
		res callbackResult
		msg = pageDone
	)
	switch errCode := q.Get("error"); {
	case errCode == "access_denied":
		res.err = fmt.Errorf("%w: 認可が拒否されました", ErrDenied)
		msg = pageDenied
	case errCode != "":
		res.err = fmt.Errorf("oauth: 認可エラー（%s）", errCode)
		msg = pageDenied
	case q.Get("code") == "":
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	default:
		res.code = q.Get("code")
	}

	writePage(w, msg)
	select {
	case c.result <- res:
	default: // 2 回目以降のコールバックは無視する
	}
}

// writePage は外部リソースを使わない簡素な完了ページを返す。
func writePage(w http.ResponseWriter, msg string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Connection", "close")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><title>Hitomado</title>
<style>body{font-family:sans-serif;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;background:#f5f5f5;color:#222}main{background:#fff;padding:2rem 2.5rem;border-radius:8px;box-shadow:0 1px 4px rgba(0,0,0,.2);max-width:28rem}h1{font-size:1.2rem;margin:0 0 .75rem}p{margin:0;line-height:1.6}</style>
</head><body><main><h1>Hitomado</h1><p>%s</p></main></body></html>
`, html.EscapeString(msg))
	// 書き込み失敗（クライアントの切断）は認可結果の受け取りに影響しない。
}
