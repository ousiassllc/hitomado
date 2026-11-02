// Package palette はアカウント色のパレットキーを定義する。
// フロントエンドの src/lib/palette.ts と同じキー・同じ順序を保つこと。
package palette

import "slices"

var keys = []string{"red", "orange", "amber", "green", "teal", "cyan", "blue", "indigo", "purple", "pink"}

// Keys はパレットキーを定義順に返す（呼び出し側が変更しても内部に影響しないコピー）。
func Keys() []string { return slices.Clone(keys) }

// IsValid は key がパレットのキーかを返す。
func IsValid(key string) bool { return slices.Contains(keys, key) }

// NextUnused は used に無い最初の色を返す。全色が使用済みなら使用回数が最も少ない
// （同数なら定義順で先の）色を返し、先頭から循環させる。パレット外のキーは無視する。
func NextUnused(used []string) string {
	count := make(map[string]int, len(keys))
	for _, u := range used {
		if IsValid(u) {
			count[u]++
		}
	}
	best := keys[0]
	for _, k := range keys {
		if count[k] < count[best] {
			best = k
		}
	}
	return best
}
