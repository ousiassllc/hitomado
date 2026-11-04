// Package update は GitHub Releases での新バージョン確認（NF-D03）と、Windows 版の更新ファイルの
// ダウンロード・検証・入れ替えの指示を提供する。
package update

import "context"

// Asset は Release に添付されたファイル。
type Asset struct {
	Name string
	URL  string // browser_download_url（HTTPS）
	Size int64  // バイト数（GitHub が返す値。不明は 0）
}

// Release は最新リリース。
type Release struct {
	Version string // 先頭の v を除いたもの（例: 0.1.0）
	URL     string // リリースページ
	Assets  []Asset
}

// Checker は最新リリースの取得。
type Checker interface {
	Latest(ctx context.Context) (Release, error)
}
