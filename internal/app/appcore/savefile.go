package appcore

import (
	"os"
	"path/filepath"
)

// WriteFileReplacing は path と同じディレクトリの一時ファイルに書き、成功したら path へ置き換える。
// どの失敗経路でも一時ファイルは残さず、path に元からあったファイルを壊さない。
func WriteFileReplacing(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".hitomado-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	// CreateTemp は 0600 で作るので、既存ファイルの権限（無ければ 0644）に揃える。
	mode := os.FileMode(0o644)
	if fi, statErr := os.Stat(path); statErr == nil {
		mode = fi.Mode().Perm()
	}
	if err = tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
