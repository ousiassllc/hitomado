//go:build !android

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// dataDir は %APPDATA%\Hitomado（他 OS では os.UserConfigDir 配下）。
func dataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	dir := filepath.Join(base, "Hitomado")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	return dir, nil
}
