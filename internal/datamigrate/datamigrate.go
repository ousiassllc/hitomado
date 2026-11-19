// Package datamigrate は、アプリのデータフォルダの名前変更（Hitomado → Nefos Inbox。#402）に伴う引っ越しを行う。
package datamigrate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// DirName は現在のデータフォルダ名（%APPDATA%\Nefos Inbox、Android は files/Nefos Inbox）。
	DirName = "Nefos Inbox"
	// LegacyDirName は v0.1.46 までのデータフォルダ名。
	LegacyDirName = "Hitomado"
)

// Result は Migrate が何をしたか。
type Result int

const (
	// NotNeeded は何もしなかった（新しい保存先が既にある、または古い保存先が無い）。
	NotNeeded Result = iota
	// Moved は古い保存先の名前を新しい保存先へ変えた（中身は同じ。古い保存先は残らない）。
	Moved
	// Copied は名前の変更ができず（ファイルが使用中など）、中身を新しい保存先へ写した。古い保存先は残してある。
	Copied
)

func (r Result) String() string {
	switch r {
	case Moved:
		return "moved"
	case Copied:
		return "copied"
	default:
		return "not_needed"
	}
}

// Migrate は dir（新しい保存先）がなく、同じ親フォルダの下に古い保存先（LegacyDirName）があるときだけ、
// 中身を dir へ移す。dir が既にあれば何もしない（古い保存先には触れない）。
//
// まず古い保存先の名前を変える（同じ親の中なので一瞬で終わり、途中の状態が無い）。名前を変えられなければ、
// 隣の作業用フォルダへ写し、写し終えてから dir へ名前を変える。どの段階で失敗しても、古い保存先は消さず、
// dir には半端な中身を残さない（作業用フォルダは片づける）。
func Migrate(dir string) (Result, error) {
	return migrate(dir, filepath.Join(filepath.Dir(dir), LegacyDirName), osOps{})
}

type ops interface {
	Rename(oldpath, newpath string) error
	OpenFile(name string, flag int, perm fs.FileMode) (io.ReadWriteCloser, error)
}

type osOps struct{}

func (osOps) Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
func (osOps) OpenFile(name string, flag int, perm fs.FileMode) (io.ReadWriteCloser, error) {
	return os.OpenFile(name, flag, perm) //nolint:gosec // 呼び出し元が決めたデータフォルダ内のパス
}

func migrate(dir, legacy string, o ops) (Result, error) {
	if _, err := os.Lstat(dir); err == nil {
		return NotNeeded, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return NotNeeded, fmt.Errorf("datamigrate: %w", err)
	}
	info, err := os.Lstat(legacy)
	if errors.Is(err, fs.ErrNotExist) {
		return NotNeeded, nil
	}
	if err != nil {
		return NotNeeded, fmt.Errorf("datamigrate: %w", err)
	}
	if !info.IsDir() {
		return NotNeeded, nil
	}

	renameErr := o.Rename(legacy, dir)
	if renameErr == nil {
		return Moved, nil
	}
	if raced(dir, legacy) {
		return NotNeeded, nil
	}

	tmp := dir + ".migrating"
	_ = os.RemoveAll(tmp) // 前回の失敗で残った作業用フォルダ（この名前はこの処理だけが作る）
	if err := copyTree(o, legacy, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return NotNeeded, fmt.Errorf("datamigrate: copy %s: %w", LegacyDirName, errors.Join(err, renameErr))
	}
	if err := o.Rename(tmp, dir); err != nil {
		_ = os.RemoveAll(tmp)
		if raced(dir, legacy) {
			return NotNeeded, nil
		}
		return NotNeeded, fmt.Errorf("datamigrate: rename: %w", err)
	}
	return Copied, nil
}

// raced は、同じ引っ越しを別のプロセス（Android のアプリ本体とヘッドレス同期）が先に終えたかを返す。
func raced(dir, legacy string) bool {
	if _, err := os.Lstat(dir); err != nil {
		return false
	}
	_, err := os.Lstat(legacy)
	return errors.Is(err, fs.ErrNotExist)
}

// copyTree は src の中身（通常のファイルとフォルダだけ）を dst へ写す。それ以外（リンクなど）があれば失敗にする。
func copyTree(o ops, src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o700)
		case info.Mode().IsRegular():
			return copyFile(o, path, target, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported file %s (%s)", rel, info.Mode().Type())
		}
	})
}

func copyFile(o ops, src, dst string, perm fs.FileMode) (err error) {
	in, err := o.OpenFile(src, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := o.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm|0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}
