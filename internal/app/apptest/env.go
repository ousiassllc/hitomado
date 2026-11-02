package apptest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/ousiassllc/hitomado/internal/ai"
	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/threadcache"
	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/palette"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

// Epoch はフェイク時計の初期時刻でありテストデータの基準時刻。
var Epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// Env はテスト用に組み立てた Deps と各フェイクへの参照。
type Env struct {
	Deps *app.Deps

	Store    store.Store
	Gmail    *FakeGmail
	Flow     *FakeFlow
	Files    *FakeFiles
	Browser  *FakeBrowser
	Paths    *FakePaths
	Notifier *FakeNotifier
	Events   *FakeEmitter
	Sync     *FakeSync
	Update   *FakeUpdate
	Secrets  *secrets.Memory
	Clock    *clock.Fake

	defaultClient string // AddAccount が使う OAuth クライアントの行 ID
}

// NewDeps は実 SQLite＋フェイクで組んだ Deps を返す。AI は呼ぶとエラーを返す既定なので、必要なテストが差し替える。
func NewDeps(tb testing.TB) *app.Deps { return NewEnv(tb).Deps }

// NewEnv は NewDeps と同じ組み立てで、フェイクへの参照も返す。
func NewEnv(tb testing.TB) *Env {
	tb.Helper()
	clk := clock.NewFake(Epoch)
	st, err := store.Open(context.Background(), filepath.Join(tb.TempDir(), "h.db"), store.WithClock(clk))
	if err != nil {
		tb.Fatalf("apptest: store.Open: %v", err)
	}
	tb.Cleanup(func() { _ = st.Close() })

	e := &Env{
		Store: st, Gmail: NewFakeGmail(), Flow: NewFakeFlow(), Files: &FakeFiles{}, Browser: &FakeBrowser{}, Paths: &FakePaths{},
		Notifier: &FakeNotifier{}, Events: NewFakeEmitter(), Update: &FakeUpdate{},
		Secrets: secrets.NewMemory(), Clock: clk,
	}
	e.Sync = NewFakeSync(st, e.Gmail)
	e.Deps = &app.Deps{
		Store: st, Gmail: e.Gmail, Secrets: e.Secrets, OAuth: e.Flow, Sync: e.Sync, Notifier: e.Notifier,
		Update: e.Update, Events: e.Events, Clock: e.Clock, Files: e.Files, Browser: e.Browser, Paths: e.Paths,
		AI: func(string, string, string, string) (ai.Provider, error) {
			return nil, errors.New("apptest: Deps.AI が未設定です")
		},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DataDir: tb.TempDir(), Version: "0.1.0",
		Threads: threadcache.New(e.Gmail, threadcache.DefaultCapacity),
	}
	return e
}

// AddOAuthClient は OAuth クライアントと Client Secret（"secret-<clientID>"）を登録する。
func (e *Env) AddOAuthClient(tb testing.TB, clientID string) store.OAuthClient {
	tb.Helper()
	c := store.OAuthClient{ID: store.NewID(), Name: "client " + clientID, ClientID: clientID, CreatedAt: e.Clock.Now(), UpdatedAt: e.Clock.Now()}
	if err := e.Store.CreateOAuthClient(context.Background(), c); err != nil {
		tb.Fatalf("apptest: CreateOAuthClient: %v", err)
	}
	if err := e.Secrets.Set(secrets.OAuthClientSecretKey(c.ID), "secret-"+clientID); err != nil {
		tb.Fatalf("apptest: secrets.Set: %v", err)
	}
	return c
}

// AddAccount はアカウントを作り（既定の OAuth クライアントを共用。未色指定は未使用色を割当）、
// リフレッシュトークン（"refresh-<email>"）を secrets に保存し、FakeGmail の Profile を登録する。Sync.Start は呼ばない。
func (e *Env) AddAccount(tb testing.TB, email string) store.Account {
	tb.Helper()
	ctx := context.Background()
	if e.defaultClient == "" {
		e.defaultClient = e.AddOAuthClient(tb, "default-client").ID
	}
	existing, err := e.Store.ListAccounts(ctx)
	if err != nil {
		tb.Fatalf("apptest: ListAccounts: %v", err)
	}
	used := make([]string, len(existing))
	for i, a := range existing {
		used[i] = a.Color
	}
	a := store.Account{
		ID: store.NewID(), OAuthClientID: e.defaultClient, Email: email, DisplayName: "name " + email,
		Color: palette.NextUnused(used), NotifyEnabled: true, Status: store.StatusActive, SortOrder: len(existing),
		CreatedAt: e.Clock.Now(), UpdatedAt: e.Clock.Now(),
	}
	if err := e.Store.CreateAccount(ctx, a); err != nil {
		tb.Fatalf("apptest: CreateAccount: %v", err)
	}
	if err := e.Secrets.Set(secrets.RefreshTokenKey(a.ID), "refresh-"+email); err != nil {
		tb.Fatalf("apptest: secrets.Set: %v", err)
	}
	e.Gmail.mu.Lock()
	e.Gmail.Profiles[a.ID] = gmail.Profile{Email: email, HistoryID: "1"}
	e.Gmail.mu.Unlock()
	return a
}

// PutThread は FakeGmail にスレッドを置き、同じ内容を Store へ反映する（同期済みの状態を作る）。
func (e *Env) PutThread(tb testing.TB, accountID, threadID string, msgs ...gmail.MessageFull) gmail.ThreadFull {
	tb.Helper()
	tf := gmail.ThreadFull{ID: threadID, HistoryID: "h-" + threadID, Messages: msgs}
	e.Gmail.mu.Lock()
	e.Gmail.Threads[Key(accountID, threadID)] = tf
	e.Gmail.mu.Unlock()
	if _, err := e.Store.ApplyThread(context.Background(), SnapshotFromMeta(accountID, MetaOf(tf))); err != nil {
		tb.Fatalf("apptest: ApplyThread: %v", err)
	}
	return tf
}

// PutRemoteThread は FakeGmail にだけスレッドを置く（Store に未同期。検索・追加取得のテスト用）。
func (e *Env) PutRemoteThread(accountID, threadID string, msgs ...gmail.MessageFull) {
	e.Gmail.mu.Lock()
	defer e.Gmail.mu.Unlock()
	e.Gmail.Threads[Key(accountID, threadID)] = gmail.ThreadFull{ID: threadID, HistoryID: "h-" + threadID, Messages: msgs}
}

// Msg はテスト用のメッセージを作る。件名は "subject <threadID>"、差出人は sender@example.com、本文は "<p>body</p>"。
// at は InternalDate。受信箱のスレッドは labels に "INBOX"、未読は "UNREAD" を渡す。
func Msg(id, threadID string, at time.Time, labels ...string) gmail.MessageFull {
	return gmail.MessageFull{
		MessageMeta: gmail.MessageMeta{
			ID: id, ThreadID: threadID, InternalDate: at, LabelIDs: labels, Snippet: "snippet " + id,
			From:    model.Address{Name: "Sender", Email: "sender@example.com"},
			To:      []model.Address{{Email: "me@example.com"}},
			Subject: "subject " + threadID,
		},
		MessageID: "<" + id + "@example.com>",
		BodyHTML:  "<p>body</p>",
	}
}

func isNotFound(err error) bool { return errors.Is(err, gmail.ErrNotFound) }
