package gmailtests

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

// 利用上限に当たったアカウントでは、あとから始まる別の呼び出しも、最初の送信の前に待ちが終わるのを待つ（上限中に投げ足して延ばさない。#392）。
func TestCallWaitsWhileAccountIsCoolingDown(t *testing.T) {
	var (
		mu    sync.Mutex
		waits []time.Duration
	)
	e := newTestEnvWith(t, func(o *gmail.Options) {
		o.Sleep = func(ctx context.Context, d time.Duration) error {
			mu.Lock()
			waits = append(waits, d)
			mu.Unlock()
			return ctx.Err() // 待ちは即時に返す
		}
	})
	var calls atomic.Int32
	e.f.Handle("GET "+apiBase+"/profile", func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) <= 3 {
			writeJSON(w, 403, apiError(403, "rateLimitExceeded", "Quota exceeded for quota metric 'Total Query Cost' and limit 'Units per minute per user'"))
			return
		}
		writeJSON(w, 200, map[string]any{"emailAddress": "a@example.com", "historyId": "1"})
	})
	_, err := e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	mu.Lock()
	retried := len(waits)
	mu.Unlock()
	require.Equal(t, 3, retried, "上限エラーのたびに待ってやり直す（自動で成功する）")

	_, err = e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, waits, 4, "2 回目の呼び出しは、最初の送信の前にアカウントの待ちが終わるのを待つ")
	assert.Greater(t, waits[3], 5*time.Second, "直前の失敗で伸ばした待ち（8〜16 秒）の残り")
	assert.LessOrEqual(t, waits[3], 16*time.Second)
}
