package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/logcore"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

func TestSyncOnceWithoutDatabaseDoesNothing(t *testing.T) {
	dir := t.TempDir()
	res, err := syncOnce(context.Background(), Options{DataDir: dir, Secrets: secrets.NewMemory()})
	require.NoError(t, err)
	assert.Empty(t, res.Accounts)
	_, statErr := os.Stat(filepath.Join(dir, DBFileName))
	assert.True(t, os.IsNotExist(statErr), "アカウント未登録のときに DB を作らない")
}

func TestSyncOnceWithRealStoreReportsMissingTokenAsNeedsReauth(t *testing.T) {
	// 実際の SQLite・Gmail クライアントで組み立てる。資格情報が無い（鍵の喪失などで読めない）アカウントは、
	// 通信せずに「要再接続」になり、DB の状態も更新される。
	dir := t.TempDir()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(dir, DBFileName))
	require.NoError(t, err)
	client := store.OAuthClient{ID: store.NewID(), Name: "c", ClientID: "cid"}
	require.NoError(t, st.CreateOAuthClient(ctx, client))
	acct := store.Account{ID: store.NewID(), OAuthClientID: client.ID, Email: "a@example.com", DisplayName: "A", Color: "#000000", NotifyEnabled: true, Status: store.StatusActive}
	require.NoError(t, st.CreateAccount(ctx, acct))
	require.NoError(t, st.Close())

	res, err := SyncOnce(ctx, Options{DataDir: dir, Secrets: secrets.NewMemory()})
	require.NoError(t, err)
	assert.Equal(t, []AccountResult{{ID: acct.ID, Email: "a@example.com", Status: StatusNeedsReauth}}, res.Accounts)
	assert.Empty(t, res.NewMail)

	st, err = store.Open(ctx, filepath.Join(dir, DBFileName))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	got, err := st.GetAccount(ctx, acct.ID)
	require.NoError(t, err)
	assert.Equal(t, store.StatusNeedsReauth, got.Status)

	// 次回以降は再接続待ちのアカウントとして、同期せずに結果だけ返す。
	res, err = SyncOnce(ctx, Options{DataDir: dir, Secrets: secrets.NewMemory()})
	require.NoError(t, err)
	assert.Equal(t, StatusNeedsReauth, res.Accounts[0].Status)
}

func TestRunReturnsEncodedFailureForUnopenableDatabase(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, DBFileName), []byte("not a database"), 0o600))
	out := Run(Options{DataDir: dir, Secrets: secrets.NewMemory()})
	m := decode(t, out)
	assert.Equal(t, false, m["ok"])
	assert.Equal(t, "failed", m["error"])
}

// ヘッドレス同期（Worker）が再接続待ちに切り替えたことは、理由（トークンが読めない）と経路（origin=headless）つきで
// アプリのログに残る。トークン・メールアドレスは載せない（#369）。
func TestSyncOnceLogsReauthReasonWithHeadlessOrigin(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(dir, DBFileName))
	require.NoError(t, err)
	client := store.OAuthClient{ID: store.NewID(), Name: "c", ClientID: "cid"}
	require.NoError(t, st.CreateOAuthClient(ctx, client))
	acct := store.Account{ID: store.NewID(), OAuthClientID: client.ID, Email: "a@example.com", DisplayName: "A", Color: "#000000", NotifyEnabled: true, Status: store.StatusActive}
	require.NoError(t, st.CreateAccount(ctx, acct))
	require.NoError(t, st.Close())

	// 鍵が失われたストア（Android）。ErrKeyLost は「無い」とも読めるが、理由は別に残す。
	_, err = SyncOnce(ctx, Options{DataDir: dir, Secrets: keyLostStore{secrets.NewMemory()}})
	require.NoError(t, err)

	files, err := filepath.Glob(filepath.Join(dir, logcore.DirName, "*"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	var rec map[string]any
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // t.TempDir() の下のログファイル
		require.NoError(t, err)
		for _, line := range strings.Split(string(b), "\n") {
			var r map[string]any
			if json.Unmarshal([]byte(line), &r) == nil && r["msg"] == "account needs reauth" {
				rec = r
			}
		}
	}
	require.NotNil(t, rec, "再接続待ちへの切り替えがログに残る")
	assert.Equal(t, "headless", rec["origin"])
	assert.Equal(t, "secret_key_lost", rec["reason"])
	assert.Equal(t, "sync", rec["trigger"])
	assert.Equal(t, acct.ID, rec["account"])
	assert.NotContains(t, fmt.Sprint(rec), "a@example.com")
}

// keyLostStore は Keystore の鍵が失われた状態の Store。
type keyLostStore struct{ secrets.Store }

func (keyLostStore) Get(string) (string, error) { return "", secrets.ErrKeyLost }
