// Package update は GitHub Releases での新バージョン確認（NF-D03）を提供する。
package update

import "context"

// Release は最新リリース。
type Release struct {
	Version string // 先頭の v を除いたもの（例: 0.1.0）
	URL     string // リリースページ
}

// Checker は最新リリースの取得。
type Checker interface {
	Latest(ctx context.Context) (Release, error)
}
