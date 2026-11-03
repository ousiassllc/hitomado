//go:build android

package platform

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Open は Android の Intent（ACTION_VIEW）で URL を開く。Wails の app.Browser.OpenURL は
// Android では xdg-open を探して失敗するため使わない（試作 #81 で確認）。
func (b *Browser) Open(url string) error {
	application.Android.OpenURL(url)
	return nil
}

// OpenPath は Android では未対応。アプリのフォルダを OS の標準アプリで開く経路が無い（FileProvider＋Intent が要る）。
func (p *Paths) OpenPath(string) error {
	return errors.New("open path: not supported on Android")
}
