package logging

import (
	"archive/zip"
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	var b bytes.Buffer
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	require.NoError(t, os.WriteFile(path, b.Bytes(), 0o600))
}

func line(at time.Time, level, msg string) string {
	return fmt.Sprintf(`{"time":%q,"level":%q,"msg":%q,"app_version":"1","k":"v"}`, at.Format(timeLayout), level, msg)
}

func TestReadEntriesFiltersByLevelPeriodAndLimit(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 3, 9, 0, 0, 0, time.FixedZone("JST", 9*3600))
	writeLines(t, filepath.Join(dir, FileName),
		"old text format line that is not json",
		line(base, "INFO", "one"),
		line(base.Add(time.Minute), "WARN", "two"),
		line(base.Add(2*time.Minute), "ERROR", "three"),
		line(base.Add(3*time.Minute), "ERROR", "four"),
		"{broken json",
	)

	all, err := ReadEntries(dir, Query{})
	require.NoError(t, err)
	require.Len(t, all, 4)
	assert.Equal(t, "one", all[0].Message, "時刻の昇順")
	assert.Equal(t, "v", all[0].Attrs["k"])
	assert.NotContains(t, all[0].Attrs, "time")

	errs, err := ReadEntries(dir, Query{MinLevel: slog.LevelError})
	require.NoError(t, err)
	assert.Equal(t, []string{"three", "four"}, messages(errs))

	last, err := ReadEntries(dir, Query{Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"three", "four"}, messages(last), "新しい方から Limit 件")

	win, err := ReadEntries(dir, Query{Since: base.Add(time.Minute), Until: base.Add(2 * time.Minute)})
	require.NoError(t, err)
	assert.Equal(t, []string{"two", "three"}, messages(win))
}

func TestReadEntriesSpansRotatedFilesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	writeLines(t, filepath.Join(dir, "hitomado-2026-11-01T00-00-00.000.log"), line(base.Add(-48*time.Hour), "ERROR", "oldest"))
	writeLines(t, filepath.Join(dir, "hitomado-2026-11-02T00-00-00.000.log"), line(base.Add(-24*time.Hour), "ERROR", "older"))
	writeLines(t, filepath.Join(dir, FileName), line(base, "ERROR", "newest"))

	got, err := ReadEntries(dir, Query{})
	require.NoError(t, err)
	assert.Equal(t, []string{"oldest", "older", "newest"}, messages(got))

	got, err = ReadEntries(dir, Query{Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, []string{"newest"}, messages(got))
}

func TestReadEntriesMissingDirIsEmpty(t *testing.T) {
	got, err := ReadEntries(filepath.Join(t.TempDir(), "none"), Query{})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestReadEntriesReturnsWhatWasWrittenThroughNew(t *testing.T) {
	dir := t.TempDir()
	l, closeFn, err := New(dir, slog.LevelInfo, false, Meta{Version: "2"})
	require.NoError(t, err)
	l.Error("sync failed", "account_id", "01HX", "err", "boom")
	require.NoError(t, closeFn())

	got, err := ReadEntries(dir, Query{MinLevel: slog.LevelError})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "sync failed", got[0].Message)
	assert.Equal(t, "ERROR", got[0].Level)
	assert.Equal(t, "01HX", got[0].Attrs["account_id"])
	assert.Equal(t, "2", got[0].Attrs["app_version"])
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{"": slog.LevelInfo, "INFO": slog.LevelInfo, "warn": slog.LevelWarn, "Error": slog.LevelError, "debug": slog.LevelDebug} {
		got, err := ParseLevel(in)
		require.NoError(t, err)
		assert.Equal(t, want, got, in)
	}
	_, err := ParseLevel("loud")
	assert.Error(t, err)
}

func TestExportZipContainsAllLogFiles(t *testing.T) {
	dir := t.TempDir()
	writeLines(t, filepath.Join(dir, FileName), `{"a":1}`)
	writeLines(t, filepath.Join(dir, "hitomado-2026-11-01T00-00-00.000.log"), `{"b":2}`)
	writeLines(t, filepath.Join(dir, "other.txt"), "not a log")

	var buf bytes.Buffer
	require.NoError(t, ExportZip(dir, &buf))
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	assert.ElementsMatch(t, []string{FileName, "hitomado-2026-11-01T00-00-00.000.log"}, names)
}

func messages(es []Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Message
	}
	return out
}
