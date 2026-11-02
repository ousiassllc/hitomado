package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/time/rate"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/logging"
)

// フロントのエラー報告の上限。画面側の暴走でログが埋まらないよう、1 秒に 1 件・同時 30 件まで受ける。
const (
	clientErrorBurst   = 30
	clientErrorMessage = 1000 // runes
	clientErrorStack   = 8000 // runes
	clientErrorShort   = 64   // code・accountId の上限（runes）
)

// LogsDirName はデータフォルダの下のログフォルダ名。
const LogsDirName = "logs"

// LogService はフロントのエラーの記録・ログフォルダを開く・ログの書き出し（バインディング仕様 §8）。
type LogService struct {
	d  *Deps
	mu sync.Mutex
	// limiter は error／unhandledrejection／vue 用、bindingLimiter は binding 用。想定内の binding の警告が
	// 続いても、本物のエラーの報告が捨てられないよう分ける。
	limiter, bindingLimiter *rate.Limiter
}

// NewLogService は LogService を作る。
func NewLogService(d *Deps) *LogService {
	return &LogService{
		d: d, limiter: rate.NewLimiter(rate.Every(time.Second), clientErrorBurst),
		bindingLimiter: rate.NewLimiter(rate.Every(time.Second), clientErrorBurst),
	}
}

// ReportClientError はフロントの未処理例外・Promise の reject・バインディング呼び出しの失敗を Go のログに記録する。
// 値は logging の伏せ処理を通り、メッセージ・スタックは長さを切る。上限を超えた報告は黙って捨てる（エラーを返さない）。
func (s *LogService) ReportClientError(_ context.Context, e dto.ClientError) error {
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
		return validation("kind が正しくありません。")
	}
	s.mu.Lock()
	ok := lim.AllowN(s.d.Clock.Now(), 1)
	s.mu.Unlock()
	if !ok {
		return nil
	}
	// 伏せ処理（Scrub）を先に通してから切る（アドレスやトークンの途中で切って断片が残らないように）。
	logger(s.d).Log(context.Background(), level, "frontend error",
		"source", "frontend", "kind", e.Kind,
		"code", clip(e.Code, clientErrorShort), "accountId", clip(e.AccountID, clientErrorShort),
		"message", clip(e.Message, clientErrorMessage),
		"origin", clip(e.Source, clientErrorMessage),
		"stack", clip(e.Stack, clientErrorStack),
	)
	return nil
}

func clip(s string, n int) string { return truncateRunes(logging.Scrub(s), n) }

// OpenLogFolder はログフォルダを OS のファイルマネージャーで開く。
func (s *LogService) OpenLogFolder(_ context.Context) error {
	dir := logDir(s.d)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return apperr.Wrap(apperr.Internal, "ログフォルダを作成できません。", err)
	}
	if err := s.d.Paths.OpenPath(dir); err != nil {
		return apperr.Wrap(apperr.Internal, "ログフォルダを開けません。", err)
	}
	return nil
}

// ExportLogs は保存先を選ばせ、ログ（現行とローテーション済み）を zip にして書き出す。キャンセルは Saved=false。
func (s *LogService) ExportLogs(ctx context.Context) (dto.ExportLogsResult, error) {
	name := "hitomado-logs-" + s.d.Clock.Now().Format("20060102-150405") + ".zip"
	path, ok, err := s.d.Files.SaveFile(ctx, name)
	if err != nil {
		return dto.ExportLogsResult{}, apperr.FromInfra(err, "")
	}
	if !ok {
		return dto.ExportLogsResult{}, nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // 利用者が保存ダイアログで選んだパス
	if err != nil {
		return dto.ExportLogsResult{}, apperr.Wrap(apperr.AttachmentIO, "ログを書き出せません。", err)
	}
	if err := logging.ExportZip(logDir(s.d), f); err != nil {
		_ = f.Close()
		_ = os.Remove(path) // 途中までの zip を残さない
		return dto.ExportLogsResult{}, apperr.Wrap(apperr.AttachmentIO, "ログを書き出せません。", err)
	}
	if err := f.Close(); err != nil {
		return dto.ExportLogsResult{}, apperr.Wrap(apperr.AttachmentIO, "ログを書き出せません。", err)
	}
	return dto.ExportLogsResult{Saved: true, Path: path}, nil
}

// logDir はデータフォルダの下のログフォルダ。
func logDir(d *Deps) string { return filepath.Join(d.DataDir, LogsDirName) }

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
