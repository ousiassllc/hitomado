package syncertests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

// lockedBuf は複数のゴルーチンから書かれるログの出力先。
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

// records は書かれた JSON Lines のうち msg が一致するレコードを返す。
func (l *lockedBuf) records(msg string) []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(l.b.String()), "\n") {
		var rec map[string]any
		if json.Unmarshal([]byte(line), &rec) == nil && rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

// 実際のログの出力（伏せ処理を通る）を取るハーネス。
func newLoggedHarness(t *testing.T) (*harness, *lockedBuf) {
	t.Helper()
	buf := &lockedBuf{}
	next := logging.NewWriter(buf, slog.LevelDebug, logging.Meta{Version: "test"}).Handler()
	return newHarnessLogged(t, next, func(*synctest.Store) {}), buf
}

const reauthMsg = "account needs reauth"

// 再接続待ちへ切り替えるとき、理由・契機・切り替え前の状態をログに残す（#369）。アカウントは ID だけで、メールアドレスは載せない。
func TestNeedsReauthTransitionLogsReason(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		wantReason string
		wantMsg    string
	}{
		{"invalid_grant", gmail.WithReason(fmt.Errorf("x: %w", gmail.ErrNeedsReauth), gmail.ReasonInvalidGrant), "invalid_grant", syncer.MsgTokenRevoked},
		{"api 401", gmail.WithReason(fmt.Errorf("x: %w", gmail.ErrNeedsReauth), gmail.ReasonAPIUnauthorized), "api_401", "再接続が必要です"},
		{"secret not found", gmail.WithReason(fmt.Errorf("x: %w", gmail.ErrNeedsReauth), gmail.ReasonSecretNotFound), "secret_not_found", "再接続が必要です"},
		{"secret key lost", gmail.WithReason(fmt.Errorf("x: %w", gmail.ErrNeedsReauth), gmail.ReasonSecretKeyLost), "secret_key_lost", "再接続が必要です"},
		{"client rejected", gmail.WithReason(fmt.Errorf("x: %w", gmail.ErrClientRejected), gmail.ReasonClientRejected), "client_rejected", "OAuth クライアントの設定（Client ID／Secret）を確認してください"},
		{"reason unknown", fmt.Errorf("x: %w", gmail.ErrNeedsReauth), "", "再接続が必要です"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, buf := newLoggedHarness(t)
			h.account("a", true, synced("1"))
			h.gm.Edit("a", func(a *synctest.Script) { a.HistoryErr = tc.err })

			h.s.Start("a")
			h.waitStatus("a", "needs_reauth", 1)

			assert.Equal(t, tc.wantMsg, h.st.Account("a").StatusMessage)
			recs := buf.records(reauthMsg)
			require.Len(t, recs, 1)
			assert.Equal(t, "WARN", recs[0]["level"])
			assert.Equal(t, "a", recs[0]["account"])
			assert.Equal(t, tc.wantReason, recs[0]["reason"])
			assert.Equal(t, "sync", recs[0]["trigger"])
			assert.Equal(t, store.StatusActive, recs[0]["prev_status"])
			assert.NotContains(t, recs[0], "since_last_sync_s", "同期できた記録が無ければ載せない")
			assert.NotContains(t, fmt.Sprint(recs[0]), "@example.com", "メールアドレスは載せない")
		})
	}
}

// 利用者の操作（受信箱の続きの取得など）で再接続待ちになるときは、契機が action になる。
func TestNeedsReauthFromActionLogsReason(t *testing.T) {
	h, buf := newLoggedHarness(t)
	h.account("a", true, store.SyncState{InitialSyncDone: true, HistoryID: "1", InboxPageToken: "p2"})
	h.gm.Edit("a", func(a *synctest.Script) {
		a.ListErr = gmail.WithReason(fmt.Errorf("x: %w", gmail.ErrNeedsReauth), gmail.ReasonScopeInsufficient)
	})

	_, err := h.s.FetchMore(t.Context(), "a")
	require.ErrorIs(t, err, gmail.ErrNeedsReauth)

	recs := buf.records(reauthMsg)
	require.Len(t, recs, 1)
	assert.Equal(t, "scope_insufficient", recs[0]["reason"])
	assert.Equal(t, "action", recs[0]["trigger"])
}

// すでに再接続待ちのアカウントで利用者の操作が再び認可エラーになっても、切り替えではないのでログを出さない（初回の 1 件だけ）。
func TestNeedsReauthFromActionLogsOnlyOnTransition(t *testing.T) {
	h, buf := newLoggedHarness(t)
	h.account("a", true, store.SyncState{InitialSyncDone: true, HistoryID: "1", InboxPageToken: "p2"})
	h.gm.Edit("a", func(a *synctest.Script) {
		a.ListErr = gmail.WithReason(fmt.Errorf("x: %w", gmail.ErrNeedsReauth), gmail.ReasonInvalidGrant)
	})

	_, err := h.s.FetchMore(t.Context(), "a")
	require.ErrorIs(t, err, gmail.ErrNeedsReauth)
	require.Len(t, buf.records(reauthMsg), 1)
	require.Equal(t, store.StatusNeedsReauth, h.st.Account("a").Status)

	_, err = h.s.FetchMore(t.Context(), "a")
	require.ErrorIs(t, err, gmail.ErrNeedsReauth)
	assert.Len(t, buf.records(reauthMsg), 1, "すでに needs_reauth なら 2 回目は記録しない")
}

// 一時的な失敗（オフライン）は再接続待ちにならないので、このログも出ない。
func TestOfflineDoesNotLogNeedsReauth(t *testing.T) {
	h, buf := newLoggedHarness(t)
	h.account("a", true, synced("1"))
	h.gm.Edit("a", func(a *synctest.Script) { a.HistoryErr = fmt.Errorf("x: %w", gmail.ErrOffline) })

	h.s.Start("a")
	h.waitStatus("a", "error", 1)

	assert.Empty(t, buf.records(reauthMsg))
	assert.Equal(t, store.StatusError, h.st.Account("a").Status)
}

// 直近に同期できた時刻からの経過秒を載せる。直前まで同期できていたのに失効したのか、長く使っていなかったのかを読み分けられる。
func TestNeedsReauthLogsSecondsSinceLastSync(t *testing.T) {
	h, buf := newLoggedHarness(t)
	lastOK := h.clk.Now().Add(-90 * time.Second)
	h.account("a", true, store.SyncState{InitialSyncDone: true, HistoryID: "1", LastSyncedAt: lastOK})
	h.gm.Edit("a", func(a *synctest.Script) {
		a.HistoryErr = gmail.WithReason(fmt.Errorf("x: %w", gmail.ErrNeedsReauth), gmail.ReasonInvalidGrant)
	})

	h.s.Start("a")
	h.waitStatus("a", "needs_reauth", 1)

	recs := buf.records(reauthMsg)
	require.Len(t, recs, 1)
	assert.InDelta(t, 90, recs[0]["since_last_sync_s"], 0)
}
