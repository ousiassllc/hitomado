//go:build windows

package appcore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
)

func TestMarkInternetZoneWritesZoneIdentifier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.pdf")
	require.NoError(t, appcore.WriteFileReplacing(path, []byte("data")))
	require.NoError(t, appcore.MarkInternetZone(path))

	b, err := os.ReadFile(path + ":Zone.Identifier") //nolint:gosec // テストの一時ファイル
	require.NoError(t, err)
	assert.Contains(t, string(b), "[ZoneTransfer]")
	assert.Contains(t, string(b), "ZoneId=3")
	body, err := os.ReadFile(path) //nolint:gosec // テストの一時ファイル
	require.NoError(t, err)
	assert.Equal(t, "data", string(body), "本体は変わらない")
}
