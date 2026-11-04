//go:build android

package platform

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ousiassllc/hitomado/internal/exportfile"
)

// Open は Android の Intent（ACTION_VIEW）で URL を開く。Wails の app.Browser.OpenURL は
// Android では xdg-open を探して失敗するため使わない（試作 #81 で確認）。
func (b *Browser) Open(url string) error {
	application.Android.OpenURL(url)
	return nil
}

// OpenPath はファイルを既定のアプリで開く（ACTION_VIEW + FileProvider。Issue #85）。
// 開けるのはアプリのログのディレクトリ（files/Hitomado/logs。FileProvider が公開する範囲）の中のファイルだけで、
// 範囲外は Java 側が拒否する。フォルダは開けない（アプリ専用の領域を表示するファイルマネージャーの経路が無い。
// exportfile.ErrFolderNotSupported）。開けるアプリが無ければ exportfile.ErrNoApp。
func (p *Paths) OpenPath(path string) error {
	mime, err := exportfile.OpenRequest(path)
	if err != nil {
		return fmt.Errorf("open path: %w", err)
	}
	if _, err := callJava("documentOpen", map[string]string{"path": path, "mime": mime}); err != nil {
		return fmt.Errorf("open path: %w", err)
	}
	return nil
}
