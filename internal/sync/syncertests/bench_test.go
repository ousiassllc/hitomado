package syncertests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

// 同期の性能の計測（Android の #90。docs/environment/android.md §17）。実行は
//
//	go test -run '^$' -bench 'InitialSync' -benchtime=1x ./internal/sync/syncertests
//
// 初回同期は INBOX の最新 200 スレッド（initialPageSize）のメタデータを取得して SQLite に保存する。

const benchThreads = 200

// benchGmailLookup は gmail.AccountLookup（どのアカウントも同じ OAuth 資格情報）。
type benchGmailLookup struct{}

func (benchGmailLookup) OAuthCredentials(context.Context, string) (string, string, error) {
	return "bench-client-id", "bench-row", nil
}

// newBenchGmailServer は Gmail API とトークンエンドポイントを兼ねる偽サーバー。
// 実 API と同じ形（threads.list・threads.get〈metadata〉・profile・labels）の JSON を n スレッド分返す。
func newBenchGmailServer(b *testing.B, n int) *httptest.Server {
	b.Helper()
	const base = "/gmail/v1/users/me"
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600})
	})
	mux.HandleFunc(base+"/profile", func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]any{"emailAddress": "bench@example.com", "historyId": "1"})
	})
	mux.HandleFunc(base+"/labels", func(w http.ResponseWriter, _ *http.Request) {
		write(w, map[string]any{"labels": []map[string]any{{"id": "INBOX", "name": "INBOX", "type": "system"}, {"id": "UNREAD", "name": "UNREAD", "type": "system"}}})
	})
	mux.HandleFunc(base+"/threads", func(w http.ResponseWriter, _ *http.Request) {
		threads := make([]map[string]any, n)
		for i := range threads {
			threads[i] = map[string]any{"id": fmt.Sprintf("t%d", i), "historyId": "1"}
		}
		write(w, map[string]any{"threads": threads})
	})
	mux.HandleFunc(base+"/threads/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, base+"/threads/")
		hdr := func(k, v string) map[string]any { return map[string]any{"name": k, "value": v} }
		write(w, map[string]any{"id": id, "historyId": "1", "messages": []map[string]any{{
			"id": "m-" + id, "threadId": id, "labelIds": []string{"INBOX", "UNREAD"}, "snippet": "bench snippet " + id,
			"internalDate": "1767225600000",
			"payload": map[string]any{"headers": []map[string]any{
				hdr("From", "Sender <sender@example.com>"), hdr("To", "bench@example.com"),
				hdr("Subject", "bench subject "+id), hdr("Date", "Thu, 1 Jan 2026 00:00:00 +0000"),
			}},
		}}})
	})
	srv := httptest.NewServer(mux)
	b.Cleanup(srv.Close)
	return srv
}

func newBenchStore(b *testing.B) (store.Store, string) {
	b.Helper()
	st, err := store.Open(b.Context(), filepath.Join(b.TempDir(), "bench.db"))
	require.NoError(b, err)
	b.Cleanup(func() { _ = st.Close() })
	oc := store.OAuthClient{ID: store.NewID(), Name: "client", ClientID: "cid"}
	require.NoError(b, st.CreateOAuthClient(b.Context(), oc))
	a := store.Account{ID: store.NewID(), OAuthClientID: oc.ID, Email: "bench@example.com", DisplayName: "Bench", Color: "red", Status: store.StatusActive}
	require.NoError(b, st.CreateAccount(b.Context(), a))
	return st, a.ID
}

func runInitialSync(b *testing.B, mkGmail func(b *testing.B, accountID string) gmail.Client) {
	b.Helper()
	for range b.N {
		b.StopTimer()
		st, accountID := newBenchStore(b)
		s := syncer.New(syncer.Deps{Store: st, Gmail: mkGmail(b, accountID), Notifier: &synctest.Notifier{}, Events: &synctest.Events{}, Clock: clock.System{}})
		b.StartTimer()
		res := s.Refresh(b.Context(), nil)
		b.StopTimer()
		s.Shutdown()
		require.Len(b, res, 1)
		require.NoError(b, res[0].Err)
		rows, err := st.ListInbox(b.Context(), store.InboxFilter{Limit: benchThreads + 1})
		require.NoError(b, err)
		require.Len(b, rows, benchThreads)
	}
}

// BenchmarkInitialSyncHTTP は実の gmail.Client を偽の HTTP サーバーに向けて初回同期する。
// クライアントの本番どおりのクォータ制御（250 units/秒、threads.get は 10 units → 約 25 スレッド/秒）を含むため、
// 200 スレッドでは待ちが支配的になる（実 Gmail に対しても同じ上限）。
func BenchmarkInitialSyncHTTP(b *testing.B) {
	srv := newBenchGmailServer(b, benchThreads)
	runInitialSync(b, func(b *testing.B, accountID string) gmail.Client {
		sec := secrets.NewMemory()
		require.NoError(b, sec.Set(secrets.RefreshTokenKey(accountID), "rt"))
		require.NoError(b, sec.Set(secrets.OAuthClientSecretKey("bench-row"), "cs"))
		c, err := gmail.NewClient(gmail.Options{
			Secrets: sec, Accounts: benchGmailLookup{}, Endpoint: srv.URL + "/", TokenURL: srv.URL + "/token", Clock: clock.System{},
		})
		require.NoError(b, err)
		return c
	})
}

// BenchmarkInitialSyncStore はクォータ制御とネットワークを除いた、同期エンジンと実 SQLite の時間
// （メモリ上のフェイクの Gmail。取得したメタデータの変換・保存・集計だけ）。
func BenchmarkInitialSyncStore(b *testing.B) {
	runInitialSync(b, func(b *testing.B, accountID string) gmail.Client {
		g := synctest.NewGmail()
		g.Edit(accountID, func(s *synctest.Script) {
			s.ProfileHist = "1"
			s.Threads = map[string]gmail.ThreadMeta{}
			var stubs []gmail.ThreadStub
			for i := range benchThreads {
				id := fmt.Sprintf("t%d", i)
				stubs = append(stubs, gmail.ThreadStub{ID: id})
				s.Threads[id] = thread(id, msg("m-"+id, id, "INBOX", "UNREAD"))
			}
			s.Pages = map[string]gmail.ThreadList{"": {Threads: stubs}}
		})
		return g
	})
}
