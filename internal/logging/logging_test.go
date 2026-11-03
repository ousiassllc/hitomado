package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWritesJSONLinesWithVersionOSAndZonedTime(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	l, closeFn, err := New(dir, slog.LevelInfo, false, Meta{Version: "9.9.9"})
	require.NoError(t, err)
	l.Info("sync done", "account_id", "01HX")
	l.Debug("hidden")
	require.NoError(t, closeFn())

	b, err := os.ReadFile(filepath.Join(dir, FileName)) //nolint:gosec // テストの一時ディレクトリ配下
	require.NoError(t, err)
	assert.NotContains(t, string(b), "hidden", "Info 未満は出さない")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(b), &rec), "1 行 1 レコードの JSON")
	assert.Equal(t, "sync done", rec["msg"])
	assert.Equal(t, "01HX", rec["account_id"])
	assert.Equal(t, "9.9.9", rec["app_version"])
	assert.Equal(t, runtime.GOOS+"/"+runtime.GOARCH, rec["os"])
	ts, ok := rec["time"].(string)
	require.True(t, ok)
	parsed, err := time.Parse(timeLayout, ts)
	require.NoError(t, err, "タイムゾーンのオフセットつきで読める: %s", ts)
	assert.WithinDuration(t, time.Now(), parsed, time.Minute)
}

func TestNewAppliesRotationLimits(t *testing.T) {
	// ローテーションの設定（5MB・5 世代・30 日）が docs と食い違わないよう固定する。
	assert.Equal(t, 5, maxSizeMB)
	assert.Equal(t, 5, maxBackups)
	assert.Equal(t, 30, maxAgeDays)
}

func TestLogPanicRecordsValueAndStack(t *testing.T) {
	var buf bytes.Buffer
	l := NewWriter(&buf, slog.LevelInfo, Meta{Version: "1"})
	func() {
		defer Recover(l, "worker")
		panic(errors.New("boom for refresh_token=abc123"))
	}()
	var rec map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec))
	assert.Equal(t, "ERROR", rec["level"])
	assert.Equal(t, "worker", rec["where"])
	assert.Contains(t, rec["stack"], "TestLogPanicRecordsValueAndStack", "スタックトレースに発生元が残る")
	assert.NotContains(t, buf.String(), "abc123", "panic の値に混ざった秘密は伏せる")
}

func TestRecoverToReturnsError(t *testing.T) {
	var buf bytes.Buffer
	l := NewWriter(&buf, slog.LevelInfo, Meta{})
	run := func() (err error) {
		defer RecoverTo(l, "run", &err)
		var p *struct{ n int }
		p.n++ // nil ポインタの参照
		return nil
	}
	err := run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "panic in run")
	assert.Contains(t, buf.String(), `"msg":"panic"`)
}
