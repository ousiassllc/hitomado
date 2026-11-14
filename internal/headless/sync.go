package headless

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ousiassllc/hitomado/internal/app/logcore"
	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
	syncer "github.com/ousiassllc/hitomado/internal/sync"
)

// DBFileName はデータフォルダの下の SQLite ファイル名。
const DBFileName = "hitomado.db"

// Timeout は 1 回の同期の上限。WorkManager のジョブの実行時間の上限（約 10 分）より短くする。
const Timeout = 5 * time.Minute

// Options は SyncOnce の入力。
type Options struct {
	DataDir string        // アプリのデータフォルダ（run.go の dataDir と同じ）
	Secrets secrets.Store // リフレッシュトークン・Client Secret の取得元（Android は SecureStore。Activity 不要）
	Version string        // ログに記す版
	Clock   clock.Clock   // nil は実時間
}

// SyncOnce は全アカウントを 1 回だけ同期して、見つけた新着とアカウントごとの結果を返す。
// データベースがまだ無ければ（アカウント未登録）何もしない。プロセスにアプリ本体が動いているときは Gate の規則に従う。
func SyncOnce(ctx context.Context, o Options) (Result, error) {
	return gate.Run(ctx, func(ctx context.Context) (Result, error) { return syncOnce(ctx, o) })
}

// Run は Timeout 付きで SyncOnce を実行し、結果を Encode した JSON で返す。panic も失敗の JSON にする
// （cgo の export から panic が漏れるとプロセスごと落ちる）。
func Run(o Options) (out string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("headless sync panic", "panic", fmt.Sprint(r))
			out = Encode(Result{}, errors.New("panic"))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	return Encode(SyncOnce(ctx, o))
}

func syncOnce(ctx context.Context, o Options) (Result, error) {
	dbPath := filepath.Join(o.DataDir, DBFileName)
	if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
		return Result{}, nil // 初回起動前。アカウントは無い（DB を作らない）
	}
	logger, closeLog, err := logging.New(filepath.Join(o.DataDir, logcore.DirName), slog.LevelInfo, false, logging.Meta{Version: o.Version})
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = closeLog() }()
	// アプリ本体と同じログファイルに書くので、どちらの経路のログかを origin で区別できるようにする（再接続待ちの原因の切り分け）。
	logger = logging.WithOrigin(logger, logging.OriginHeadless)
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return Result{}, fmt.Errorf("headless: open database: %w", err)
	}
	defer func() { _ = st.Close() }()
	clk := o.Clock
	if clk == nil {
		clk = clock.System{}
	}
	gm, err := gmail.NewClient(gmail.Options{Secrets: o.Secrets, Accounts: StoreLookup{Store: st}, Clock: clk, Logger: logger})
	if err != nil {
		return Result{}, fmt.Errorf("headless: gmail client: %w", err)
	}
	return syncWith(ctx, st, gm, clk, logger)
}

// syncWith は組み立て済みの store・gmail で全アカウントを 1 回同期する（ファイルや資格情報に触れない純粋な部分）。
// 同期は通常のスケジューラ（60 秒ループと同じ cycle）の手動更新で行い、通知の代わりに新着を集める。
// 終わったらスケジューラを止めて、実行中の周期の終了を待つ。
func syncWith(ctx context.Context, st store.Store, gm gmail.Client, clk clock.Clock, logger *slog.Logger) (Result, error) {
	col := &collector{}
	sched := syncer.New(syncer.Deps{Store: st, Gmail: gm, Notifier: col, Clock: clk, Logger: logger})
	accounts, err := RefreshAll(ctx, st, sched)
	sched.Shutdown()
	return Result{NewMail: col.collected(), Accounts: accounts}, err
}
