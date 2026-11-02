// Package gmailtests は gmail.Client を公開 API だけで検証するテスト（偽 Gmail サーバー gmailtest に向ける）。
// internal/gmail 直下の行数上限のため、非公開の実装に触れないテストはここに置く。
package gmailtests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/gmail/gmailtest"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

const (
	testAccount  = "acc-1"
	testRowID    = "oc-1"
	testClientID = "client-id.apps.googleusercontent.com"
	apiBase      = "/gmail/v1/users/me"
	sendUpload   = "/upload/gmail/v1/users/me/messages/send"
	draftsUpload = "/upload/gmail/v1/users/me/drafts"
)

var (
	writeJSON = gmailtest.WriteJSON
	apiError  = gmailtest.APIError
)

// fakeLookup は gmail.AccountLookup のフェイク。
type fakeLookup struct{}

func (fakeLookup) OAuthCredentials(_ context.Context, accountID string) (string, string, error) {
	if accountID != testAccount {
		return "", "", fmt.Errorf("unknown account %s", accountID)
	}
	return testClientID, testRowID, nil
}

type testEnv struct {
	f *gmailtest.Server
	c gmail.Client
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	return newTestEnvWith(t, nil)
}

// newTestEnvWith は偽サーバーに向けたクライアントを作る。customize で Options を書き換えられる（nil 可）。
// バックオフ待ちは即時に返す。
func newTestEnvWith(t *testing.T, customize func(*gmail.Options)) *testEnv {
	t.Helper()
	f := gmailtest.New(t)
	store := secrets.NewMemory()
	require.NoError(t, store.Set(secrets.RefreshTokenKey(testAccount), "rt-1"))
	require.NoError(t, store.Set(secrets.OAuthClientSecretKey(testRowID), "client-secret"))
	opts := gmail.Options{
		Secrets: store, Accounts: fakeLookup{}, Endpoint: f.URL() + "/", TokenURL: f.URL() + "/token",
		Clock: clock.NewFake(time.Now()),
		Sleep: func(ctx context.Context, _ time.Duration) error { return ctx.Err() },
	}
	if customize != nil {
		customize(&opts)
	}
	c, err := gmail.NewClient(opts)
	require.NoError(t, err)
	return &testEnv{f: f, c: c}
}

func hdr(name, value string) map[string]any { return map[string]any{"name": name, "value": value} }
