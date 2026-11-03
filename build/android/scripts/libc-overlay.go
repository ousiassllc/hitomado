//go:build ignore

// libc-overlay.go は android/amd64 の Go ビルド用に、旧式 syscall を読み替えるパッチを当てた
// modernc.org/libc の複製と、それを使う -modfile を作る（docs/environment/android.md §10）。
//
// 使い方（リポジトリのルートで。ホストの GOOS/GOARCH のまま実行する）:
//
//	go run build/android/scripts/libc-overlay.go
//	GOOS=android GOARCH=amd64 CGO_ENABLED=1 go build -modfile=build/android/gen/libc-amd64/go.mod …
//
// go.mod・go.sum・モジュールキャッシュは変えない。生成物は build/android/gen/（git 管理外）の下だけ。
// （go build -overlay はモジュールキャッシュ内のファイルを置き換えられないため、-modfile の replace を使う。）
package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// パッチの対象にしている libc の版。go.mod の版が違うと失敗する（パッチを作り直す）。
const libcVersion = "v1.79.0"

const libcModule = "modernc.org/libc"

func main() {
	if err := run("build/android/libc/amd64.patch", "build/android/gen/libc-amd64"); err != nil {
		fmt.Fprintln(os.Stderr, "libc-overlay:", err)
		os.Exit(1)
	}
}

func run(patch, work string) error {
	absWork, err := filepath.Abs(work)
	if err != nil {
		return err
	}
	absPatch, err := filepath.Abs(patch)
	if err != nil {
		return err
	}
	src, err := libcDir()
	if err != nil {
		return err
	}

	// 複製の置き場: <work>/libc（パッチ済みの libc）、<work>/go.mod・go.sum（-modfile 用）。
	if err := os.RemoveAll(absWork); err != nil {
		return err
	}
	libcCopy := filepath.Join(absWork, "libc")
	if err := copyTree(src, libcCopy); err != nil {
		return err
	}
	if msg, err := exec.Command("patch", "--no-backup-if-mismatch", "-p1", "-s", "-d", libcCopy, "-i", absPatch).CombinedOutput(); err != nil {
		return fmt.Errorf("patch failed (libc changed? see docs/environment/android.md §10): %w\n%s", err, msg)
	}

	// -modfile=<x>/go.mod は <x>/go.sum を読む。go.sum はそのまま複製する（replace 先のディレクトリには go.sum 項目は要らない）。
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(absWork, name), data, 0o644); err != nil {
			return err
		}
	}
	edit := exec.Command("go", "mod", "edit", "-modfile="+filepath.Join(absWork, "go.mod"), "-replace="+libcModule+"="+libcCopy)
	if msg, err := edit.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod edit: %w\n%s", err, msg)
	}
	fmt.Printf("libc-overlay: %s %s patched for android/amd64 -> %s\n", libcModule, libcVersion, absWork)
	return nil
}

// libcDir は go.mod が選んだ libc のモジュールキャッシュ上のディレクトリを返す。版がパッチの対象と違えば失敗する。
func libcDir() (string, error) {
	b, err := exec.Command("go", "list", "-m", "-f", "{{.Version}} {{.Dir}}", libcModule).Output()
	if err != nil {
		return "", fmt.Errorf("go list -m %s: %w", libcModule, err)
	}
	version, dir, ok := strings.Cut(strings.TrimSpace(string(b)), " ")
	if !ok || dir == "" {
		return "", fmt.Errorf("go list -m %s: unexpected output %q (run `go mod download`)", libcModule, b)
	}
	if version != libcVersion {
		return "", fmt.Errorf("%s is %s in go.mod but build/android/libc/amd64.patch is for %s: "+
			"re-apply the change to the new version, regenerate the patch and update libcVersion in this file (docs/environment/android.md §10)",
			libcModule, version, libcVersion)
	}
	return dir, nil
}

// copyTree は src を dst へ複製する（テストファイルは除く）。モジュールキャッシュは読み取り専用なので、書き込める権限にする。
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if strings.HasSuffix(path, "_test.go") || !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
