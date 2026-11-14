// Package capability は実行している OS で「アプリが提供する機能」を表す。
// デスクトップ（Windows 版）は全機能、Android は MCP サーバー・トレイ・多重起動の防止・ウィンドウ状態の保存復元を提供しない
// （Issue #89）。Android のアプリ内の更新は「新しい版のお知らせとダウンロードページのリンク」だけで、ダウンロードと入れ替えはしない
// （APK は検証用で、署名が版ごとに変わり上書きインストールできない）。代わりに Android はタッチ操作中心の画面（Issue #87）を使う。
//
// 機能の有無は OS 名ではなくこの値で判断する（フロントへは PlatformService.GetCapabilities で渡す）。
// 判定の中身は OS に依存しない純粋な関数・値で、現在の OS を決めるのは current_android.go／current_other.go だけ。
package capability

// OS 名（runtime.GOOS と同じ表記）。
const OSAndroid = "android"

// Capabilities は機能ごとの有無。ゼロ値はすべて無効（設定し忘れて機能が動いてしまうことを避ける）。
// JSON はフロントへ渡す形（PlatformService.GetCapabilities）。
type Capabilities struct {
	OS                  string `json:"os"`                  // runtime.GOOS と同じ表記（表示・ログ用。機能の判断には使わない）
	SupportsMCP         bool   `json:"supportsMcp"`         // 内蔵 MCP サーバー（待ち受け・設定・トークン）
	SupportsTray        bool   `json:"supportsTray"`        // システムトレイと、× で終了せずトレイへ格納する動作
	SupportsAutoUpdate  bool   `json:"supportsAutoUpdate"`  // アプリ内の更新確認（定期確認・手動確認・新しい版のお知らせ）。ダウンロードと入れ替えは別（AppInfo.autoUpdateSupported）
	SupportsWindowState bool   `json:"supportsWindowState"` // ウィンドウの位置・サイズ・最大化の保存と復元
	SingleInstance      bool   `json:"singleInstance"`      // 多重起動の防止（2 つ目のプロセスは既存ウィンドウを前面に出して終了する）
	TouchUI             bool   `json:"touchUi"`             // タッチ操作中心の画面（幅に応じて 1 画面ずつ進む表示・44px のタップ領域）。有効なら最小ウィンドウ幅の制限も外す
	SupportsMascot      bool   `json:"supportsMascot"`      // 画面の端に残るマスコット（別ウィンドウ。#372）。Windows のみ。Android は別の方式（docs/environment/android.md）
}

// Desktop はデスクトップ OS（Windows など）の機能。MCP・トレイ・更新確認・ウィンドウ状態・多重起動の防止はすべて有効で、タッチ UI は無効（3 ペイン固定）。
// マスコットのウィンドウは Windows のみ（ほかのデスクトップ OS は未検証）。
func Desktop(os string) Capabilities {
	return Capabilities{
		OS: os, SupportsMCP: true, SupportsTray: true, SupportsAutoUpdate: true, SupportsWindowState: true, SingleInstance: true,
		SupportsMascot: os == "windows",
	}
}

// Android は Android の機能。更新確認とタッチ UI だけが有効で、MCP・トレイ・ウィンドウ状態・多重起動の防止は無効。
func Android() Capabilities {
	return Capabilities{OS: OSAndroid, SupportsAutoUpdate: true, TouchUI: true}
}
