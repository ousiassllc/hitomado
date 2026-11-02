//go:build !windows

package appcore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
)

func TestMarkInternetZoneIsNoOpOutsideWindows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.pdf")
	require.NoError(t, appcore.WriteFileReplacing(path, []byte("data")))
	require.NoError(t, appcore.MarkInternetZone(path))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "何も作らない")
}
