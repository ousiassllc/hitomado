// Package storetest は store.Store を実 SQLite（temp ファイル）に対して検証する。
package storetest

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite" // 検証用の直接接続（store が保存した行の確認）

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/store"
)

// testStore は Store と、保存内容を直接確認するための読み取り用接続。
type testStore struct {
	store.Store
	raw *sql.DB
}

// newTestStore は temp ファイルの実 SQLite を開く（時計は 2026-11-02 から始まる Fake）。
func newTestStore(t *testing.T) (*testStore, *clock.Fake) {
	t.Helper()
	clk := clock.NewFake(time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	path := filepath.Join(t.TempDir(), "h.db")
	s, err := store.Open(context.Background(), path, store.WithClock(clk))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	raw, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = raw.Close() })
	return &testStore{Store: s, raw: raw}, clk
}

// seedAccount は OAuth クライアントとアカウントを作成して返す。
func seedAccount(t *testing.T, s *testStore, email string) store.Account {
	t.Helper()
	ctx := context.Background()
	oc := store.OAuthClient{ID: store.NewID(), Name: "client " + email, ClientID: "cid-" + email}
	require.NoError(t, s.CreateOAuthClient(ctx, oc))
	a := store.Account{
		ID: store.NewID(), OAuthClientID: oc.ID, Email: email, DisplayName: "name", Color: "red",
		NotifyEnabled: true, Status: store.StatusActive,
	}
	require.NoError(t, s.CreateAccount(ctx, a))
	got, err := s.GetAccount(ctx, a.ID)
	require.NoError(t, err)
	return got
}

// msg はテスト用のメッセージスナップショット。
func msg(id string, at time.Time, labels ...string) store.MessageSnapshot {
	return store.MessageSnapshot{
		ID: id, InternalDate: at, FromName: "From " + id, FromEmail: "From-" + id + "@Example.com",
		Snippet: "snippet " + id, LabelIDs: labels,
	}
}

// snap は 1 メッセージのスレッドスナップショット。
func snap(accountID, threadID string, at time.Time, labels ...string) store.ThreadSnapshot {
	return store.ThreadSnapshot{
		AccountID: accountID, ThreadID: threadID, HistoryID: "h1", Subject: "subject " + threadID,
		Messages: []store.MessageSnapshot{msg(threadID+"-m1", at, labels...)},
	}
}

func apply(t *testing.T, s *testStore, sn store.ThreadSnapshot) store.ApplyResult {
	t.Helper()
	res, err := s.ApplyThread(context.Background(), sn)
	require.NoError(t, err)
	return res
}

func count(t *testing.T, s *testStore, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, s.raw.QueryRow(query, args...).Scan(&n))
	return n
}
