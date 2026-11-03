package logging

import (
	"archive/zip"
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

// 1 行が上限を超えても、その行だけ飛ばして後ろの行を読む。
func TestReadEntriesSkipsOversizedLineAndKeepsReading(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	huge := `{"time":"` + base.Format(timeLayout) + `","level":"ERROR","msg":"huge","pad":"` + strings.Repeat("x", maxLineBytes+10) + `"}`
	writeLines(t, filepath.Join(dir, FileName), line(base, "ERROR", "before"), huge, line(base.Add(time.Minute), "ERROR", "after"))
	got, err := ReadEntries(dir, Query{})
	require.NoError(t, err)
	assert.Equal(t, []string{"before", "after"}, messages(got))
}

// MinLevel の事前絞り込みが、WARN 以上・独自水準（ERROR+4）を取りこぼさない。
func TestReadEntriesMinLevelPrefilter(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	writeLines(t, filepath.Join(dir, FileName),
		line(base, "INFO", "i"), line(base.Add(time.Second), "WARN", "w"), line(base.Add(2*time.Second), "ERROR", "e"),
		line(base.Add(3*time.Second), "ERROR+4", "e4"))
	for lvl, want := range map[slog.Level][]string{
		slog.LevelWarn: {"w", "e", "e4"}, slog.LevelError: {"e", "e4"}, slog.LevelError + 4: {"e4"}, slog.LevelError + 8: {},
	} {
		got, err := ReadEntries(dir, Query{MinLevel: lvl})
		require.NoError(t, err)
		assert.Equal(t, want, messages(got), lvl.String())
	}
}

// 書き出しでは、伏せ処理前の古い形式のログ行に混ざった秘密・アドレスも外へ出さない。
func TestExportZipScrubsLegacyLines(t *testing.T) {
	dir := t.TempDir()
	writeLines(t, filepath.Join(dir, "hitomado-2026-10-01T00-00-00.000.log"),
		`time=2026-10-01T00:00:00.000Z level=ERROR msg="oauth failed" err="refresh_token=EVADEX1 for alice@legacy.example"`,
		`{"msg":"x","access_token":"EVADEX2"}`)
	var buf bytes.Buffer
	require.NoError(t, ExportZip(dir, &buf))
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	rc, err := zr.File[0].Open()
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	var out bytes.Buffer
	_, err = out.ReadFrom(rc)
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "EVADEX")
	assert.NotContains(t, out.String(), "alice@legacy.example")
	assert.Contains(t, out.String(), "oauth failed")
}

func TestReadEntriesReturnsNewestLimitMatches(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	// 古い側にノイズ、新しい側に 3 件の ERROR。
	var lines []string
	for i := range 3000 {
		lines = append(lines, line(base.Add(time.Duration(i)*time.Second), "INFO", "noise"))
	}
	lines = append(lines, line(base.Add(time.Hour), "ERROR", "e1"), line(base.Add(2*time.Hour), "ERROR", "e2"), line(base.Add(3*time.Hour), "ERROR", "e3"))
	writeLines(t, filepath.Join(dir, FileName), lines...)

	got, err := ReadEntries(dir, Query{Limit: 2, MinLevel: slog.LevelError})
	require.NoError(t, err)
	assert.Equal(t, []string{"e2", "e3"}, messages(got), "新しい方から Limit 件を時刻の昇順で")
}

func TestReadEntriesTakesRemainderFromOlderFile(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	writeLines(t, filepath.Join(dir, "hitomado-2026-11-02T00-00-00.000.log"),
		line(base.Add(-3*time.Hour), "ERROR", "o1"), line(base.Add(-2*time.Hour), "ERROR", "o2"), line(base.Add(-time.Hour), "ERROR", "o3"))
	writeLines(t, filepath.Join(dir, FileName), line(base, "ERROR", "n1"), line(base.Add(time.Minute), "ERROR", "n2"))

	got, err := ReadEntries(dir, Query{Limit: 4})
	require.NoError(t, err)
	assert.Equal(t, []string{"o2", "o3", "n1", "n2"}, messages(got))
}

// エラーの無い約 30MB のログ（6 ファイル分）でも get_status 相当の呼び出しが実用的な時間で返る。
// 壁時計の上限は CI の遅さを見込んで 2 秒と十分に緩くしてある（実測は通常その数十分の一）。
func TestReadEntriesErrorFreeLargeLogIsFast(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	info := line(base, "INFO", strings.Repeat("m", 200)) + "\n"
	chunk := strings.Repeat(info, 1000)
	write := func(name string, size int) {
		f, err := os.Create(filepath.Join(dir, name)) //nolint:gosec // t.TempDir 内
		require.NoError(t, err)
		for written := 0; written < size; written += len(chunk) {
			_, err = f.WriteString(chunk)
			require.NoError(t, err)
		}
		require.NoError(t, f.Close())
	}
	write(FileName, 5<<20)
	for i := 1; i <= 5; i++ {
		write(fmt.Sprintf("hitomado-2026-10-0%d.log", i), 5<<20)
	}

	start := time.Now()
	got, err := ReadEntries(dir, Query{Limit: 1, MinLevel: slog.LevelError})
	elapsed := time.Since(start)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Less(t, elapsed, 2*time.Second)
}
