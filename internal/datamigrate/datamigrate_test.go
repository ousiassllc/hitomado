package datamigrate

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // テスト用の一時フォルダ内のパス
	require.NoError(t, err)
	return string(b)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// legacyTree は旧保存先（DB・設定・ログ）を作り、(親フォルダ, 新しい保存先のパス) を返す。
func legacyTree(t *testing.T) (parent, dir string) {
	t.Helper()
	parent = t.TempDir()
	legacy := filepath.Join(parent, LegacyDirName)
	write(t, filepath.Join(legacy, "hitomado.db"), "db-bytes")
	write(t, filepath.Join(legacy, "logs", "hitomado.log"), "log-line")
	write(t, filepath.Join(legacy, "updates", "x.tmp"), "x")
	return parent, filepath.Join(parent, DirName)
}

func TestMigrateMovesLegacyDir(t *testing.T) {
	parent, dir := legacyTree(t)

	got, err := Migrate(dir)
	require.NoError(t, err)
	assert.Equal(t, Moved, got)
	assert.Equal(t, "db-bytes", read(t, filepath.Join(dir, "hitomado.db")))
	assert.Equal(t, "log-line", read(t, filepath.Join(dir, "logs", "hitomado.log")))
	assert.Equal(t, "x", read(t, filepath.Join(dir, "updates", "x.tmp")))
	assert.False(t, exists(filepath.Join(parent, LegacyDirName)), "引っ越したあとに旧保存先は残らない")

	// 2 回目は何もしない。
	got, err = Migrate(dir)
	require.NoError(t, err)
	assert.Equal(t, NotNeeded, got)
}

func TestMigrateLeavesEverythingWhenNewDirExists(t *testing.T) {
	parent, dir := legacyTree(t)
	write(t, filepath.Join(dir, "hitomado.db"), "newer-db")

	got, err := Migrate(dir)
	require.NoError(t, err)
	assert.Equal(t, NotNeeded, got)
	assert.Equal(t, "newer-db", read(t, filepath.Join(dir, "hitomado.db")), "新しい保存先は上書きしない")
	assert.Equal(t, "db-bytes", read(t, filepath.Join(parent, LegacyDirName, "hitomado.db")), "旧保存先にも触れない")
}

func TestMigrateNoLegacyDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), DirName)
	got, err := Migrate(dir)
	require.NoError(t, err)
	assert.Equal(t, NotNeeded, got)
	assert.False(t, exists(dir), "新規利用者のフォルダはここでは作らない")
}

// 名前の変更ができないとき（ファイルが使用中など）は、写してから新しい保存先へ名前を変える。旧保存先は残す。
func TestMigrateCopiesWhenRenameFails(t *testing.T) {
	parent, dir := legacyTree(t)
	legacy := filepath.Join(parent, LegacyDirName)

	got, err := migrate(dir, legacy, &fakeOps{failRenameOf: legacy})
	require.NoError(t, err)
	assert.Equal(t, Copied, got)
	assert.Equal(t, "db-bytes", read(t, filepath.Join(dir, "hitomado.db")))
	assert.Equal(t, "log-line", read(t, filepath.Join(dir, "logs", "hitomado.log")))
	assert.Equal(t, "db-bytes", read(t, filepath.Join(legacy, "hitomado.db")), "写したときは旧保存先を残す")
	assert.False(t, exists(dir+".migrating"))
}

// 写している途中で失敗しても、旧保存先は無傷で、新しい保存先には半端な中身が残らない。
func TestMigrateFailureKeepsLegacyData(t *testing.T) {
	parent, dir := legacyTree(t)
	legacy := filepath.Join(parent, LegacyDirName)

	got, err := migrate(dir, legacy, &fakeOps{failRenameOf: legacy, failOpen: "hitomado.log"})
	require.Error(t, err)
	assert.Equal(t, NotNeeded, got)
	assert.Equal(t, "db-bytes", read(t, filepath.Join(legacy, "hitomado.db")))
	assert.Equal(t, "log-line", read(t, filepath.Join(legacy, "logs", "hitomado.log")))
	assert.False(t, exists(dir), "新しい保存先ができていない（次の起動でやり直せる）")
	assert.False(t, exists(dir+".migrating"), "作業用フォルダを片づける")

	// 原因が消えた次の起動では、最後まで移る。
	got, err = Migrate(dir)
	require.NoError(t, err)
	assert.Equal(t, Moved, got)
	assert.Equal(t, "log-line", read(t, filepath.Join(dir, "logs", "hitomado.log")))
}

// 最後の名前の変更（作業用フォルダ → 新しい保存先）で失敗したときも同じ。
func TestMigrateFinalRenameFailureKeepsLegacyData(t *testing.T) {
	parent, dir := legacyTree(t)
	legacy := filepath.Join(parent, LegacyDirName)

	_, err := migrate(dir, legacy, &fakeOps{failRenameOf: legacy, failRenameOf2: dir + ".migrating"})
	require.Error(t, err)
	assert.Equal(t, "db-bytes", read(t, filepath.Join(legacy, "hitomado.db")))
	assert.False(t, exists(dir))
	assert.False(t, exists(dir+".migrating"))
}

// 前回の失敗で作業用フォルダが残っていても、それを使わずに作り直す。
func TestMigrateDiscardsStaleStagingDir(t *testing.T) {
	parent, dir := legacyTree(t)
	legacy := filepath.Join(parent, LegacyDirName)
	write(t, filepath.Join(dir+".migrating", "stale.txt"), "stale")

	got, err := migrate(dir, legacy, &fakeOps{failRenameOf: legacy})
	require.NoError(t, err)
	assert.Equal(t, Copied, got)
	assert.False(t, exists(filepath.Join(dir, "stale.txt")))
	assert.Equal(t, "db-bytes", read(t, filepath.Join(dir, "hitomado.db")))
}

func TestMigrateRefusesIrregularFiles(t *testing.T) {
	parent, dir := legacyTree(t)
	legacy := filepath.Join(parent, LegacyDirName)
	if err := os.Symlink(filepath.Join(legacy, "hitomado.db"), filepath.Join(legacy, "link")); err != nil {
		t.Skipf("symlink: %v", err)
	}

	_, err := migrate(dir, legacy, &fakeOps{failRenameOf: legacy})
	require.Error(t, err)
	assert.False(t, exists(dir))
	assert.True(t, exists(filepath.Join(legacy, "hitomado.db")))
}

// fakeOps は名前の変更・ファイルを開く操作を指定どおりに失敗させる。
type fakeOps struct {
	failRenameOf  string // この名前からの Rename を失敗させる
	failRenameOf2 string // 同上（2 つ目）
	failOpen      string // このファイル名（末尾）を読み取りで開くと失敗する
}

func (f *fakeOps) Rename(oldpath, newpath string) error {
	if oldpath == f.failRenameOf || (f.failRenameOf2 != "" && oldpath == f.failRenameOf2) {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errors.New("used by another process")}
	}
	return os.Rename(oldpath, newpath)
}

func (f *fakeOps) OpenFile(name string, flag int, perm fs.FileMode) (io.ReadWriteCloser, error) {
	if f.failOpen != "" && flag == os.O_RDONLY && strings.HasSuffix(name, f.failOpen) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: errors.New("access denied")}
	}
	return osOps{}.OpenFile(name, flag, perm)
}
