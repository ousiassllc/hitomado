package gmail

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/gmail/gmailtest"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

const (
	testAccount  = "acc-1"
	testRowID    = "oc-1"
	testClientID = "client-id.apps.googleusercontent.com"
)

var (
	writeJSON = gmailtest.WriteJSON
	apiError  = gmailtest.APIError
)

// fakeLookup は AccountLookup のフェイク。
type fakeLookup struct{}

func (fakeLookup) OAuthCredentials(_ context.Context, accountID string) (string, string, error) {
	if accountID != testAccount {
		return "", "", fmt.Errorf("unknown account %s", accountID)
	}
	return testClientID, testRowID, nil
}

// sleeper はバックオフ待ちを即時に返しつつ、要求された待ち時間を記録する。
type sleeper struct {
	mu    sync.Mutex
	waits []time.Duration
	// onSleep が非 nil なら各待ちで呼ぶ（キャンセルの注入用）。
	onSleep func(n int)
}

func (s *sleeper) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.waits = append(s.waits, d)
	n := len(s.waits)
	hook := s.onSleep
	s.mu.Unlock()
	if hook != nil {
		hook(n)
	}
	return ctx.Err()
}

func (s *sleeper) recorded() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.waits...)
}

type testEnv struct {
	f       *gmailtest.Server
	c       *client
	store   *secrets.Memory
	sleeper *sleeper
}

// newTestEnv は偽サーバーに向けたクライアントを作る。ジッタは上限値を返す（待ち時間を決定的にする）。
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	f := gmailtest.New(t)
	store := secrets.NewMemory()
	require.NoError(t, store.Set(secrets.RefreshTokenKey(testAccount), "rt-1"))
	require.NoError(t, store.Set(secrets.OAuthClientSecretKey(testRowID), "client-secret"))
	sl := &sleeper{}
	cl, err := NewClient(Options{
		Secrets: store, Accounts: fakeLookup{}, Endpoint: f.URL() + "/", TokenURL: f.URL() + "/token",
		Clock: clock.NewFake(time.Now()), Sleep: sl.sleep,
	})
	require.NoError(t, err)
	c := cl.(*client)
	c.jitter = func(d time.Duration) time.Duration { return d }
	return &testEnv{f: f, c: c, store: store, sleeper: sl}
}
