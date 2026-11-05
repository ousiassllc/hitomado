package oauth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/callbackpage"
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

	callbackpage.Write(w, msg)
	select {
	case c.result <- res:
	default: // 2 回目以降のコールバックは無視する
	}
}
