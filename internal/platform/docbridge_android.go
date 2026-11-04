//go:build android

package platform

import (
	"errors"
	_ "unsafe" // go:linkname

	"github.com/ousiassllc/hitomado/internal/exportfile"
)

// Wails の Java ブリッジを名前で呼ぶ関数（pkg/application/application_android.go）。application.Android の公開メソッドには
// 「任意のメソッドを名前で呼ぶ」ものが無く、保存先を選ばせる（ACTION_CREATE_DOCUMENT）・ファイルを開く（ACTION_VIEW）は
// Wails に無い操作なので、Wails 自身が SecureGet などを Java へ渡すのと同じこの関数を go:linkname で直接呼ぶ。
// WailsBridge.java の自前のメソッド（document*）は同じ「String を受け取り String を返す」形。
// Wails を上げて関数名・シグネチャが変わるとリンクで失敗する（実行時まで気づかないことはない）。
// cgo なしのビルドでは Wails 側のスタブ（常に ok=false）に解決され、call は errBridgeUnavailable を返す。
// 本体のないこの宣言には、同じディレクトリの空のアセンブリファイル（docbridge_android.s）が要る。
//
//go:linkname androidBridgeStringString github.com/wailsapp/wails/v3/pkg/application.androidBridgeStringString
func androidBridgeStringString(method, arg string) (string, bool)

var errBridgeUnavailable = errors.New("android bridge unavailable")

// callJava は WailsBridge.<method>(String) を呼び、{"ok":true,"value":…} / {"ok":false,"error":…} の value を返す。
// payload は JSON にする（ASCII のみ。JNI の NewStringUTF は標準の UTF-8 の補助文字を扱えない）。
func callJava(method string, payload any) (string, error) {
	arg, err := exportfile.ASCIIJSON(payload)
	if err != nil {
		return "", err
	}
	raw, ok := androidBridgeStringString(method, arg)
	if !ok {
		return "", errBridgeUnavailable
	}
	return exportfile.ParseEnvelope(raw)
}
