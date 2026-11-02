package mcptests

import (
	"archive/zip"
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/logging"
)

// newLogSvc は実際のログ組み立て（伏せ処理つき）の出力を buf で受ける LogService を返す。
func newLogSvc(t *testing.T) (*app.LogService, *apptest.Env, *bytes.Buffer) {
	t.Helper()
	e := apptest.NewEnv(t)
	buf := &bytes.Buffer{}
	e.Deps.Logger = logging.NewWriter(buf, slog.LevelDebug, logging.Meta{Version: "t"})
	return app.NewLogService(e.Deps), e, buf
}

// フロントから届いたエラーが Go のログに残り、メッセージに混ざった秘密・アドレスは伏せられる。
func TestReportClientErrorIsLoggedAndRedacted(t *testing.T) {
	svc, _, buf := newLogSvc(t)
	require.NoError(t, svc.ReportClientError(t.Context(), dto.ClientError{
		Kind:    "unhandledrejection",
		Message: "failed for alice.private@example.com Bearer ya29.FAKEFRONTENDTOKEN refresh_token=FAKEREFRESH",
		Stack:   "Error: x\n    at foo (app.js:1:2)",
		Source:  "app.js:1:2",
	}))
	out := buf.String()
	assert.Contains(t, out, `"msg":"frontend error"`)
	assert.Contains(t, out, `"source":"frontend"`)
	assert.Contains(t, out, `"kind":"unhandledrejection"`)
	assert.Contains(t, out, `"level":"ERROR"`)
	assert.Contains(t, out, "app.js:1:2")
	for _, secret := range []string{"alice.private@example.com", "FAKEFRONTENDTOKEN", "FAKEREFRESH"} {
		assert.NotContains(t, out, secret)
	}
}

func TestReportClientErrorBindingFailureLevels(t *testing.T) {
	svc, _, buf := newLogSvc(t)
	ctx := t.Context()
	require.NoError(t, svc.ReportClientError(ctx, dto.ClientError{Kind: "binding", Code: apperr.Offline, Message: "offline", AccountID: "01HX"}))
	assert.Contains(t, buf.String(), `"level":"WARN"`, "想定内のコードは警告")
	assert.Contains(t, buf.String(), `"code":"OFFLINE"`)
	assert.Contains(t, buf.String(), `"accountId":"01HX"`)

	buf.Reset()
	require.NoError(t, svc.ReportClientError(ctx, dto.ClientError{Kind: "binding", Code: apperr.Internal, Message: "boom"}))
	assert.Contains(t, buf.String(), `"level":"ERROR"`)
}

func TestReportClientErrorRejectsUnknownKindAndTruncates(t *testing.T) {
	svc, _, buf := newLogSvc(t)
	requireCode(t, apperr.Validation, svc.ReportClientError(t.Context(), dto.ClientError{Kind: "other", Message: "x"}))
	assert.Empty(t, buf.String())

	require.NoError(t, svc.ReportClientError(t.Context(), dto.ClientError{Kind: "error", Message: strings.Repeat("あ", 5000)}))
	assert.Less(t, buf.Len(), 8000, "メッセージは 1000 文字で切る")
}

// 画面の暴走でログが埋まらないよう、短時間の大量の報告は捨てる（時間が進めば再び受ける）。
func TestReportClientErrorIsRateLimited(t *testing.T) {
	svc, e, buf := newLogSvc(t)
	for range 100 {
		require.NoError(t, svc.ReportClientError(t.Context(), dto.ClientError{Kind: "error", Message: "loop"}))
	}
	assert.Equal(t, 30, strings.Count(buf.String(), "\n"))

	e.Clock.Advance(2 * time.Second)
	require.NoError(t, svc.ReportClientError(t.Context(), dto.ClientError{Kind: "error", Message: "later"}))
	assert.Equal(t, 31, strings.Count(buf.String(), "\n"))
}

func TestOpenLogFolderOpensDataDirLogs(t *testing.T) {
	svc, e, _ := newLogSvc(t)
	require.NoError(t, svc.OpenLogFolder(t.Context()))
	want := filepath.Join(e.Deps.DataDir, "logs")
	assert.Equal(t, []string{want}, e.Paths.Opened)
	assert.DirExists(t, want)
}

func TestExportLogsWritesZipOfLogFiles(t *testing.T) {
	svc, e, _ := newLogSvc(t)
	logDir := filepath.Join(e.Deps.DataDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(logDir, logging.FileName), []byte(`{"msg":"hi"}`+"\n"), 0o600))
	e.Files.Path = filepath.Join(t.TempDir(), "out.zip")

	res, err := svc.ExportLogs(t.Context())
	require.NoError(t, err)
	assert.Equal(t, dto.ExportLogsResult{Saved: true, Path: e.Files.Path}, res)
	assert.Contains(t, e.Files.Suggested[0], "hitomado-logs-")
	zr, err := zip.OpenReader(e.Files.Path)
	require.NoError(t, err)
	defer func() { _ = zr.Close() }()
	require.Len(t, zr.File, 1)
	assert.Equal(t, logging.FileName, zr.File[0].Name)
}

func TestExportLogsCancelWritesNothing(t *testing.T) {
	svc, e, _ := newLogSvc(t)
	e.Files.Cancel = true
	res, err := svc.ExportLogs(t.Context())
	require.NoError(t, err)
	assert.Equal(t, dto.ExportLogsResult{}, res)
}
