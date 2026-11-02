package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPragmasApplyToEveryConnection(t *testing.T) {
	ctx := context.Background()
	s, err := open(ctx, filepath.Join(t.TempDir(), "h.db"))
	require.NoError(t, err)
	defer func() { _ = s.Close() }()
	s.db.SetMaxOpenConns(3)

	// 同時に複数接続を保持して、どの接続にも PRAGMA が効いていることを確認する。
	for range 3 {
		c, err := s.db.Conn(ctx)
		require.NoError(t, err)
		defer func() { _ = c.Close() }()
		var fk, timeout, secure int
		var mode string
		require.NoError(t, c.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk))
		require.NoError(t, c.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode))
		require.NoError(t, c.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout))
		require.NoError(t, c.QueryRowContext(ctx, "PRAGMA secure_delete").Scan(&secure))
		assert.Equal(t, 1, fk)
		assert.Equal(t, 1, secure)
		assert.Equal(t, "wal", mode)
		assert.Equal(t, 5000, timeout)
	}
}

func TestEmailRange(t *testing.T) {
	from, to := emailRange("ali")
	assert.Equal(t, "ali", from)
	assert.Equal(t, "alj", to)
	_, to = emailRange("a\U0010FFFF")
	assert.Equal(t, "b", to, "最終文字が上限のときは 1 つ前の文字を進める")
	_, to = emailRange("")
	assert.Equal(t, "\U0010FFFF", to)
}
