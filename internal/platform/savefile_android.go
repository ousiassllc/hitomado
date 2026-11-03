//go:build android

package platform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// SaveFile は Android では保存ダイアログが使えない（Wails はエラーを返す）ので、アプリ専用の files/exports に保存先を決める。
// 試作（#81）。本実装は Storage Access Framework（ACTION_CREATE_DOCUMENT）か共有シート（Share）へ渡す。
func (f *Files) SaveFile(_ context.Context, suggestedName string) (string, bool, error) {
	dir := filepath.Join(application.Android.StoragePath(), "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", false, fmt.Errorf("exports dir: %w", err)
	}
	return filepath.Join(dir, suggestedName), true, nil
}
