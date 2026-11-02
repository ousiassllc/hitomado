package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWritesToLogFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	l, closeFn, err := New(dir, slog.LevelInfo, false)
	require.NoError(t, err)
	l.Info("sync done", "account_id", "01HX")
	l.Debug("hidden")
	require.NoError(t, closeFn())

	b, err := os.ReadFile(filepath.Join(dir, "hitomado.log")) //nolint:gosec // テストの一時ディレクトリ配下
	require.NoError(t, err)
	assert.Contains(t, string(b), "account_id=01HX")
	assert.NotContains(t, string(b), "hidden", "Info 未満は出さない")
}
