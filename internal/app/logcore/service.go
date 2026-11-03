// Package logcore はフロントのエラーの記録・ログフォルダを開く・ログの書き出しの実装（バインディング仕様 §8）。
// Wails のバインディング面は app.LogService で、ここへ委譲する。
package logcore

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/time/rate"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/exportfile"
	"github.com/ousiassllc/hitomado/internal/logging"
)

// フロントのエラー報告の上限。画面側の暴走でログが埋まらないよう、1 秒に 1 件・同時 30 件まで受ける。
const (
	clientErrorBurst   = 30
	clientErrorMessage = 1000 // runes
	clientErrorStack   = 8000 // runes
	clientErrorShort   = 64   // code・accountId の上限（runes）
)

// DirName はデータフォルダの下のログフォルダ名。
const DirName = "logs"

// Service はフロントのエラーの記録・ログフォルダを開く・ログの書き出し。
type Service struct {
	d  *deps.Deps
	mu sync.Mutex
	// limiter は error／unhandledrejection／vue 用、bindingLimiter は binding 用。想定内の binding の警告が
	// 続いても、本物のエラーの報告が捨てられないよう分ける。
	limiter, bindingLimiter *rate.Limiter
}

// New は Service を作る。
func New(d *deps.Deps) *Service {
	return &Service{
		d: d, limiter: rate.NewLimiter(rate.Every(time.Second), clientErrorBurst),
		bindingLimiter: rate.NewLimiter(rate.Every(time.Second), clientErrorBurst),
	}
}

// Dir はデータフォルダの下のログフォルダ。
func Dir(d *deps.Deps) string { return filepath.Join(d.DataDir, DirName) }

// ReportClientError はフロントの未処理例外・Promise の reject・バインディング呼び出しの失敗を Go のログに記録する。
// 値は logging の伏せ処理を通り、メッセージ・スタックは長さを切る。上限を超えた報告は黙って捨てる（エラーを返さない）。
func (s *Service) ReportClientError(_ context.Context, e dto.ClientError) error {
	level := slog.LevelError
	lim := s.limiter
	switch e.Kind {
	case "error", "unhandledrejection", "vue":
	case "binding":
		lim = s.bindingLimiter
		// 利用者の操作や通信で起きる想定内のエラー（OFFLINE など）は警告にする。
		if e.Code != apperr.Internal {
			level = slog.LevelWarn
		}
	default:
		return apperr.New(apperr.Validation, "kind が正しくありません。")
	}
	s.mu.Lock()
	ok := lim.AllowN(s.d.Clock.Now(), 1)
	s.mu.Unlock()
	if !ok {
		return nil
	}
	// 伏せ処理（Scrub）を先に通してから切る（アドレスやトークンの途中で切って断片が残らないように）。
	s.logger().Log(context.Background(), level, "frontend error",
		"source", "frontend", "kind", e.Kind,
		"code", clip(e.Code, clientErrorShort), "accountId", clip(e.AccountID, clientErrorShort),
		"message", clip(e.Message, clientErrorMessage),
		"origin", clip(e.Source, clientErrorMessage),
		"stack", clip(e.Stack, clientErrorStack),
	)
	return nil
}

func (s *Service) logger() *slog.Logger {
	if s.d.Logger != nil {
		return s.d.Logger
	}
	return slog.Default()
}

func clip(s string, n int) string { return truncateRunes(logging.Scrub(s), n) }

// OpenLogFolder はログフォルダを OS のファイルマネージャーで開く。
func (s *Service) OpenLogFolder(_ context.Context) error {
	dir := Dir(s.d)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return apperr.Wrap(apperr.Internal, "ログフォルダを作成できません。", err)
	}
	if err := s.d.Paths.OpenPath(dir); err != nil {
		if errors.Is(err, exportfile.ErrFolderNotSupported) { // Android: アプリ専用の領域をファイルマネージャーで開く経路が無い
			return apperr.Wrap(apperr.Unsupported, "この端末ではログフォルダを開けません。「ログを書き出す」で保存してください。", err)
		}
		return apperr.Wrap(apperr.Internal, "ログフォルダを開けません。", err)
	}
	return nil
}

// ExportLogs は保存先を選ばせ、ログ（現行とローテーション済み）を zip にして書き出す。キャンセルは Saved=false。
// Result.Path は Windows では保存したパス、Android では利用者が選んだ保存先のファイル名。
func (s *Service) ExportLogs(ctx context.Context) (dto.ExportLogsResult, error) {
	name := "hitomado-logs-" + s.d.Clock.Now().Format("20060102-150405") + ".zip"
	target, ok, err := s.d.Files.SaveFile(ctx, name)
	if err != nil {
		return dto.ExportLogsResult{}, apperr.FromInfra(err, "")
	}
	if !ok {
		return dto.ExportLogsResult{}, nil
	}
	defer target.Close()
	path := target.Path()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // 利用者が保存ダイアログで選んだパス
	if err != nil {
		return dto.ExportLogsResult{}, apperr.Wrap(apperr.AttachmentIO, "ログを書き出せません。", err)
	}
	if err := logging.ExportZip(Dir(s.d), f); err != nil {
		_ = f.Close()
		_ = os.Remove(path) // 途中までの zip を残さない
		return dto.ExportLogsResult{}, apperr.Wrap(apperr.AttachmentIO, "ログを書き出せません。", err)
	}
	if err := f.Close(); err != nil {
		return dto.ExportLogsResult{}, apperr.Wrap(apperr.AttachmentIO, "ログを書き出せません。", err)
	}
	location, err := target.Commit(ctx)
	if err != nil {
		return dto.ExportLogsResult{}, apperr.Wrap(apperr.AttachmentIO, "ログを書き出せません。", err)
	}
	return dto.ExportLogsResult{Saved: true, Path: location}, nil
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
