//go:build !android

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ousiassllc/nefos-inbox/internal/datamigrate"
)

// dataDir は %APPDATA%\Nefos Inbox（他 OS では os.UserConfigDir 配下）。
// 古い名前（%APPDATA%\Hitomado）のデータフォルダが残っていて、新しい名前が無いときは、ここで移す（#402）。
func dataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	dir := filepath.Join(base, datamigrate.DirName)
	if _, err := datamigrate.Migrate(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	return dir, nil
}
