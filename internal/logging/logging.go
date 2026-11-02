// Package logging はログ出力の組み立てを提供する。トークン・本文・件名・宛先は出力しない（NF-S10）。
// 呼び出し側はアカウントをアカウント ID でのみ記録すること。
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// New は dir/hitomado.log にローテーション付きで出力する logger を返す。
// 端末がある開発時は標準エラーにも出す。close はファイルを閉じる。
func New(dir string, level slog.Level, alsoStderr bool) (logger *slog.Logger, closeFn func() error, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}
	file := &lumberjack.Logger{
		Filename:   filepath.Join(dir, "hitomado.log"),
		MaxSize:    5, // MB
		MaxBackups: 5,
		MaxAge:     30, // days
	}
	var w io.Writer = file
	if alsoStderr {
		w = io.MultiWriter(file, os.Stderr)
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})), file.Close, nil
}
