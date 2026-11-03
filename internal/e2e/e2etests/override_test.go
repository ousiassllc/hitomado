//go:build e2e && !production

package e2etests

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/e2e"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
	"github.com/ousiassllc/hitomado/internal/synctest"
)

func openStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "h.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestOverrideSeedsOneAccountAndIsIdempotent(t *testing.T) {
	st, dir := openStore(t), t.TempDir()
	for range 2 { // DB が残った 2 回目の起動でも、アカウントは 1 件のまま
		sec, gm, err := e2e.Override(t.Context(), dir, st, secrets.NewMemory(), nil, slog.Default())
		require.NoError(t, err)

		accounts, err := st.ListAccounts(t.Context())
		require.NoError(t, err)
		require.Len(t, accounts, 1)
		assert.Equal(t, e2e.AccountEmail, accounts[0].Email)

		tok, err := sec.Get(secrets.RefreshTokenKey(e2e.AccountID))
		require.NoError(t, err)
		assert.NotEmpty(t, tok)
		list, err := gm.ListThreads(t.Context(), e2e.AccountID, gmail.ListQuery{LabelIDs: []string{"INBOX"}})
		require.NoError(t, err)
		assert.Len(t, list.Threads, e2e.ThreadCount)
	}
}

// 実エンジン（Scheduler）で初回同期すると、シードのスレッドが DB（一覧）に入る。アプリ起動時に一覧が出る流れの Go 側。
func TestSeededGmailSyncsIntoStore(t *testing.T) {
	st, dir := openStore(t), t.TempDir()
	_, gm, err := e2e.Override(t.Context(), dir, st, secrets.NewMemory(), nil, slog.Default())
	require.NoError(t, err)

	s := syncer.New(syncer.Deps{
		Store: st, Gmail: gm, Notifier: &synctest.Notifier{}, Events: &synctest.Events{}, Clock: clock.System{},
	})
	t.Cleanup(s.Shutdown)
	res := s.Refresh(t.Context(), nil)
	require.Len(t, res, 1)
	require.NoError(t, res[0].Err)

	rows, err := st.ListInbox(t.Context(), store.InboxFilter{Limit: 50})
	require.NoError(t, err)
	assert.Len(t, rows, e2e.ThreadCount)
}

func TestRecordingGmailWritesDraftText(t *testing.T) {
	st, dir := openStore(t), t.TempDir()
	_, gm, err := e2e.Override(t.Context(), dir, st, secrets.NewMemory(), nil, slog.Default())
	require.NoError(t, err)

	raw := []byte("To: a@example.com\r\nSubject: Re: x\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nhello e2e draft\r\n")
	id, err := gm.CreateDraft(t.Context(), e2e.AccountID, raw, "e2e-thread-1")
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dir, e2e.DraftsDir, id+".txt")) //nolint:gosec // テストの一時ディレクトリ内
	require.NoError(t, err)
	assert.Contains(t, string(b), "hello e2e draft")
	assert.Contains(t, string(b), "thread: e2e-thread-1")

	raw2 := []byte("To: a@example.com\r\nSubject: Re: x\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nupdated text\r\n")
	require.NoError(t, gm.UpdateDraft(t.Context(), e2e.AccountID, id, raw2))
	b, err = os.ReadFile(filepath.Join(dir, e2e.DraftsDir, id+".txt")) //nolint:gosec // テストの一時ディレクトリ内
	require.NoError(t, err)
	assert.Contains(t, string(b), "updated text")
}
