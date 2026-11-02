// Package logging はログ出力の組み立てを提供する。
//
// 出力は JSON Lines（1 行 1 レコード）で、すべてのレコードに時刻（タイムゾーンつき）・アプリの版・OS を載せる。
// トークン・パスワード・OAuth 情報・メール本文・件名・宛先は、書き込む前に伏せる（NF-S10。redact.go）。
// 呼び出し側はアカウントをアカウント ID でのみ記録すること。
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/natefinch/lumberjack.v2"
)

// ログのファイル名とローテーションの設定。
const (
	// FileName は現行のログファイル名。ローテーション済みは hitomado-<時刻>.log になる。
	FileName = "hitomado.log"

	maxSizeMB  = 5
	maxBackups = 5
	maxAgeDays = 30
)

// timeLayout はミリ秒と数値のタイムゾーンオフセットを持つ時刻の書式（UTC でも +00:00 と書く）。
const timeLayout = "2006-01-02T15:04:05.000-07:00"

// Meta はすべてのレコードに付けるアプリの情報。
type Meta struct {
	Version string
}

// New は dir/hitomado.log にローテーション付きで出力する logger を返す。
// 端末がある開発時は標準エラーにも出す。close はファイルを閉じる。
// 出力はすべて Redact を通る。
func New(dir string, level slog.Level, alsoStderr bool, meta Meta) (logger *slog.Logger, closeFn func() error, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}
	file := &lumberjack.Logger{
		Filename:   filepath.Join(dir, FileName),
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
	}
	var w io.Writer = file
	if alsoStderr {
		w = io.MultiWriter(file, os.Stderr)
	}
	return NewWriter(w, level, meta), file.Close, nil
}

// NewWriter は w に JSON Lines で出力する logger を返す（Redact 済み）。テストと New が使う。
func NewWriter(w io.Writer, level slog.Level, meta Meta) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format(timeLayout))
			}
			return a
		},
	})
	return slog.New(Redact(h)).With("app_version", meta.Version, "os", runtime.GOOS+"/"+runtime.GOARCH)
}
