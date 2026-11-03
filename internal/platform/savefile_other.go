//go:build !android

package platform

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ousiassllc/hitomado/internal/exportfile"
)

// SaveFile は保存先を選ばせる。キャンセルは ok=false。選ばれたパスへそのまま書く Target を返す。
func (f *Files) SaveFile(_ context.Context, suggestedName string) (exportfile.Target, bool, error) {
	d := f.app.Dialog.SaveFile().SetFilename(suggestedName).CanCreateDirectories(true)
	if w := f.window(); w != nil {
		d = d.AttachToWindow(w)
	}
	path, err := d.PromptForSingleSelection()
	if err != nil {
		return nil, false, fmt.Errorf("save dialog: %w", err)
	}
	if path == "" {
		return nil, false, nil
	}
	return exportfile.NewPathTarget(path), true, nil
}

// newSaver は Android の保存（exportfile.Saver）を作る。Android 以外には要らない。
func newSaver(*application.App) *exportfile.Saver { return nil }
