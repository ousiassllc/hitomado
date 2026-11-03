//go:build !android

package platform

import (
	"context"
	"fmt"
)

// SaveFile は保存先を選ばせる。キャンセルは ok=false。
func (f *Files) SaveFile(_ context.Context, suggestedName string) (string, bool, error) {
	d := f.app.Dialog.SaveFile().SetFilename(suggestedName).CanCreateDirectories(true)
	if w := f.window(); w != nil {
		d = d.AttachToWindow(w)
	}
	path, err := d.PromptForSingleSelection()
	if err != nil {
		return "", false, fmt.Errorf("save dialog: %w", err)
	}
	return path, path != "", nil
}
