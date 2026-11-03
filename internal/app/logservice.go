package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/app/logcore"
)

// LogsDirName はデータフォルダの下のログフォルダ名。
const LogsDirName = logcore.DirName

// LogService はフロントのエラーの記録・ログフォルダを開く・ログの書き出し（バインディング仕様 §8）。
// Wails のバインディング面で、実装は logcore に委譲する。
type LogService struct{ core *logcore.Service }

// NewLogService は LogService を作る。
func NewLogService(d *Deps) *LogService { return &LogService{core: logcore.New((*deps.Deps)(d))} }

// ReportClientError はフロントの未処理例外・Promise の reject・バインディング呼び出しの失敗を Go のログに記録する。
func (s *LogService) ReportClientError(ctx context.Context, e dto.ClientError) error {
	return s.core.ReportClientError(ctx, e)
}

// OpenLogFolder はログフォルダを OS のファイルマネージャーで開く。
func (s *LogService) OpenLogFolder(ctx context.Context) error { return s.core.OpenLogFolder(ctx) }

// ExportLogs は保存先を選ばせ、ログを zip にして書き出す。キャンセルは Saved=false。
func (s *LogService) ExportLogs(ctx context.Context) (dto.ExportLogsResult, error) {
	return s.core.ExportLogs(ctx)
}
