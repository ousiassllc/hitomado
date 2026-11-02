package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/pressly/goose/v3"
	"modernc.org/sqlite"
	sqlitelib "modernc.org/sqlite/lib"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/store/db"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// sqlStore は sqlc 生成コードと database/sql を包む Store 実装。
type sqlStore struct {
	db  *sql.DB
	q   *db.Queries
	now func() time.Time
}

var _ Store = (*sqlStore)(nil)

// NewID は新しい ULID（ローカル採番テーブルの主キー）を返す。
func NewID() string { return ulid.Make().String() }

// Option は Open の任意設定。
type Option func(*sqlStore)

// WithClock は更新日時・left_inbox_at などに使う時計を差し替える（既定は実時間。テスト用）。
func WithClock(c clock.Clock) Option {
	return func(s *sqlStore) { s.now = c.Now }
}

// Open は path の SQLite を開き（なければ作成）、マイグレーションを適用して Store を返す。
// マイグレーションに失敗したときはエラーを返す（呼び出し側は起動を中止する）。
func Open(ctx context.Context, path string, opts ...Option) (Store, error) {
	return open(ctx, path, opts...)
}

func open(ctx context.Context, path string, opts ...Option) (*sqlStore, error) {
	conn, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("store: open: %w", err)
	}
	if err := migrate(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	s := &sqlStore{db: conn, q: db.New(conn), now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// dsn は接続ごとに適用する PRAGMA（ER §1）と、書き込みトランザクションを即時ロックにする設定を付ける。
// 即時ロックにより、読み込み後の書き込みへの昇格による SQLITE_BUSY（busy_timeout が効かない）を避ける。
func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "secure_delete(1)") // 削除した件名・スニペットを空きページに残さない
	q.Set("_txlock", "immediate")
	return path + "?" + q.Encode()
}

func migrate(ctx context.Context, conn *sql.DB) error {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("store: migrations: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, conn, sub)
	if err != nil {
		return fmt.Errorf("store: migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

func (s *sqlStore) Close() error { return s.db.Close() }

// withTx は fn を 1 トランザクションで実行する（エラーまたは panic でロールバック）。
func (s *sqlStore) withTx(ctx context.Context, fn func(q *db.Queries) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// isConstraint は err が指定の SQLite 拡張結果コードの制約違反かを返す。
func isConstraint(err error, code int) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == code
}

func isUnique(err error) bool { return isConstraint(err, sqlitelib.SQLITE_CONSTRAINT_UNIQUE) }

// isForeignKey は外部キー違反かを返す。ON DELETE RESTRICT による拒否は SQLITE_CONSTRAINT_TRIGGER で報告される。
func isForeignKey(err error) bool {
	return isConstraint(err, sqlitelib.SQLITE_CONSTRAINT_FOREIGNKEY) || isConstraint(err, sqlitelib.SQLITE_CONSTRAINT_TRIGGER)
}

// notFoundIfNoRows は sql.ErrNoRows を ErrNotFound に変換する。
func notFoundIfNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// requireRows は更新・削除の影響行数が 0 のとき ErrNotFound を返す。
func requireRows(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func toMillis(t time.Time) int64 { return t.UTC().UnixMilli() }

func fromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
