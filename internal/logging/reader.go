package logging

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry は JSON Lines のログの 1 レコード。Attrs は time・level・msg 以外のキー（書き込み時に伏せ済み）。
type Entry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"msg"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// Query は ReadEntries の絞り込み条件。
type Query struct {
	Limit    int        // 返す最大件数（新しい方から）。0 は DefaultLimit、MaxLimit を超えない
	MinLevel slog.Level // この水準以上
	Since    time.Time  // この時刻以降（ゼロ値は指定なし）
	Until    time.Time  // この時刻以前（ゼロ値は指定なし）
}

// 件数の既定と上限。
const (
	DefaultLimit = 100
	MaxLimit     = 1000
)

// maxLineBytes は 1 行の読み取り上限（超える行は飛ばす）。
const maxLineBytes = 1 << 20

// ParseLevel は "debug"／"info"／"warn"／"error"（大文字小文字不問。空は info）を slog.Level にする。
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("unknown log level %q", s)
}

// logFiles は現行ファイルとローテーション済みのファイルを新しい順に返す。
func logFiles(dir string) ([]string, error) {
	rotated, err := filepath.Glob(filepath.Join(dir, "hitomado-*.log"))
	if err != nil {
		return nil, fmt.Errorf("glob log files: %w", err)
	}
	// ローテーション名は hitomado-<時刻>.log なので、名前の降順が新しい順。
	sort.Sort(sort.Reverse(sort.StringSlice(rotated)))
	files := make([]string, 0, len(rotated)+1)
	cur := filepath.Join(dir, FileName)
	if _, err := os.Stat(cur); err == nil {
		files = append(files, cur)
	}
	return append(files, rotated...), nil
}

// ReadEntries はログフォルダから条件に合うレコードを読み、時刻の昇順で返す（新しい方から Limit 件）。
// JSON として読めない行（旧形式のテキストなど）は飛ばす。ログフォルダが無ければ空。
func ReadEntries(dir string, q Query) ([]Entry, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)
	files, err := logFiles(dir)
	if err != nil {
		return nil, err
	}
	var newestFirst [][]Entry // ファイルごと（新しいファイルが先）
	total := 0
	for _, f := range files {
		entries, err := readFile(f, q)
		if err != nil {
			return nil, err
		}
		newestFirst = append(newestFirst, entries)
		if total += len(entries); total >= limit {
			break
		}
	}
	out := make([]Entry, 0, min(total, limit))
	for i := len(newestFirst) - 1; i >= 0; i-- {
		out = append(out, newestFirst[i]...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

func readFile(path string, q Query) ([]Entry, error) {
	f, err := os.Open(path) //nolint:gosec // logFiles が返したログフォルダ内のパス
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil // 読む間にローテーションで消えた
	}
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer func() { _ = f.Close() }()
	var out []Entry
	err = eachLine(f, func(line []byte) {
		if !q.mayMatch(line) {
			return
		}
		if e, ok := parseLine(line); ok && q.match(e) {
			out = append(out, e)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("read log: %w", err)
	}
	return out, nil
}

// eachLine は r を 1 行ずつ fn へ渡す。maxLineBytes を超える行は読み飛ばし、後ろの行は読み続ける。
func eachLine(r io.Reader, fn func(line []byte)) error {
	br := bufio.NewReaderSize(r, 64*1024)
	var buf []byte
	tooLong := false
	for {
		chunk, isPrefix, err := br.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err //nolint:wrapcheck // 呼び出し元がまとめて包む
		}
		if !tooLong {
			if len(buf)+len(chunk) > maxLineBytes {
				tooLong = true
			} else {
				buf = append(buf, chunk...)
			}
		}
		if isPrefix {
			continue
		}
		if !tooLong {
			fn(buf)
		}
		buf, tooLong = buf[:0], false
	}
}

// mayMatch は JSON を読む前の安い絞り込み。MinLevel が info より上のとき、水準名が行に無ければ読まない。
func (q Query) mayMatch(line []byte) bool {
	if q.MinLevel <= slog.LevelInfo {
		return true
	}
	for _, l := range []slog.Level{slog.LevelError, slog.LevelWarn} {
		if l >= q.MinLevel && bytes.Contains(line, []byte(`"level":"`+l.String())) {
			return true
		}
	}
	// ERROR+4 のような独自水準は水準名が "ERROR+4" で始まるので上の ERROR で拾える。WARN+n も同様。
	return q.MinLevel > slog.LevelError && bytes.Contains(line, []byte(`"level":"ERROR+`))
}

func parseLine(line []byte) (Entry, bool) {
	var raw map[string]any
	if json.Unmarshal(line, &raw) != nil {
		return Entry{}, false
	}
	ts, _ := raw[slog.TimeKey].(string)
	t, err := time.Parse(timeLayout, ts)
	if err != nil {
		return Entry{}, false
	}
	e := Entry{Time: t, Attrs: map[string]any{}}
	e.Level, _ = raw[slog.LevelKey].(string)
	e.Message, _ = raw[slog.MessageKey].(string)
	for k, v := range raw {
		switch k {
		case slog.TimeKey, slog.LevelKey, slog.MessageKey:
		default:
			e.Attrs[k] = v
		}
	}
	return e, true
}

func (q Query) match(e Entry) bool {
	var lvl slog.Level
	if lvl.UnmarshalText([]byte(e.Level)) != nil || lvl < q.MinLevel {
		return false
	}
	if !q.Since.IsZero() && e.Time.Before(q.Since) {
		return false
	}
	return q.Until.IsZero() || !e.Time.After(q.Until)
}

// ExportZip はログフォルダのログファイル（現行とローテーション済み）を zip にして w へ書く。
// 現行形式のログは書き込み時に伏せ済みだが、古い版が書いたログ（伏せ処理なし）が残っていても外へ出ないよう、
// 書き出すときにも 1 行ずつ Scrub を通す。
func ExportZip(dir string, w io.Writer) error {
	files, err := logFiles(dir)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	for _, path := range files {
		if err := addToZip(zw, path); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("close zip: %w", err)
	}
	return nil
}

func addToZip(zw *zip.Writer, path string) error {
	f, err := os.Open(path) //nolint:gosec // logFiles が返したログフォルダ内のパス
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer func() { _ = f.Close() }()
	dst, err := zw.Create(filepath.Base(path))
	if err != nil {
		return fmt.Errorf("zip entry: %w", err)
	}
	err = eachLine(f, func(line []byte) {
		_, _ = io.WriteString(dst, Scrub(string(line))+"\n")
	})
	if err != nil {
		return fmt.Errorf("zip copy: %w", err)
	}
	return nil
}
