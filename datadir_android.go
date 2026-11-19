//go:build android

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ousiassllc/nefos-inbox/internal/datamigrate"
)

// dataDir は Android のアプリ専用領域（activity.getFilesDir()）配下。
// Android では HOME が / で os.UserConfigDir が書き込めない場所（/.config）を返すため使わない。
// application.New のあとでないと Java ブリッジが使えない。
// 古い名前（files/Hitomado）のデータフォルダが残っていて、新しい名前（files/Nefos Inbox）が無いときは、ここで移す（#402）。
// Android にはインストーラーが無いので、この引っ越しだけで対応する。
func dataDir() (string, error) {
	base := application.Android.StoragePath()
	if base == "" {
		return "", fmt.Errorf("android storage path is empty")
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
