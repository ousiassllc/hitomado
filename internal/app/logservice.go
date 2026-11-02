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
)

// LogsDirName はデータフォルダの下のログフォルダ名。
const LogsDirName = "logs"

// LogService はフロントのエラーの記録・ログフォルダを開く・ログの書き出し（バインディング仕様 §8）。
type LogService struct {
	d       *Deps
	mu      sync.Mutex
	limiter *rate.Limiter
}

// NewLogService は LogService を作る。
func NewLogService(d *Deps) *LogService {
	return &LogService{d: d, limiter: rate.NewLimiter(rate.Every(time.Second), clientErrorBurst)}
}

// ReportClientError はフロントの未処理例外・Promise の reject・バインディング呼び出しの失敗を Go のログに記録する。
// 値は logging の伏せ処理を通り、メッセージ・スタックは長さを切る。上限を超えた報告は黙って捨てる（エラーを返さない）。
func (s *LogService) ReportClientError(_ context.Context, e dto.ClientError) error {
	s.mu.Lock()
	ok := s.limiter.AllowN(s.d.Clock.Now(), 1)
	s.mu.Unlock()
	if !ok {
		return nil
	}
	level := slog.LevelError
	switch e.Kind {
	case "error", "unhandledrejection", "vue":
	case "binding":
		// 利用者の操作や通信で起きる想定内のエラー（OFFLINE など）は警告にする。
		if e.Code != apperr.Internal {
			level = slog.LevelWarn
		}
	default:
		return validation("kind が正しくありません。")
	}
	logger(s.d).Log(context.Background(), level, "frontend error",
		"source", "frontend", "kind", e.Kind, "code", e.Code, "accountId", e.AccountID,
		"message", truncateRunes(e.Message, clientErrorMessage),
		"origin", truncateRunes(e.Source, clientErrorMessage),
		"stack", truncateRunes(e.Stack, clientErrorStack),
	)
	return nil
}

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
