//go:build android

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// dataDir は Android のアプリ専用領域（activity.getFilesDir()）配下。
// Android では HOME が / で os.UserConfigDir が書き込めない場所（/.config）を返すため使わない。
// application.New のあとでないと Java ブリッジが使えない。
func dataDir() (string, error) {
	base := application.Android.StoragePath()
	if base == "" {
		return "", fmt.Errorf("android storage path is empty")
	}
	dir := filepath.Join(base, "Hitomado")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	return dir, nil
}
