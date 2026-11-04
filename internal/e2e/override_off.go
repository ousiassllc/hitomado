//go:build !e2e || production

// Package e2e は Android の E2E（build/android/e2e）用の「テスト用起動モード」。ビルドタグ e2e のときだけ、
// Gmail を差し替え可能なフェイクにし、資格情報をメモリ上に置き、最小のシード（アカウント 1 件・スレッド数件）を入れる。
//
// タグが無い（リリースを含む通常のビルド）、または production タグと一緒のときは Override が何もせず、
// このパッケージのフェイク・シードはバイナリに入らない（docs/environment/android.md §17）。
package e2e

import (
	"context"
	"log/slog"

	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/store"
)

// Enabled は E2E 用ビルド（タグ e2e かつ production でない）のとき true。
const Enabled = false

// Override は E2E モードでないとき、渡された secrets・gmail をそのまま返す。
func Override(_ context.Context, _ string, _ store.Store, sec secrets.Store, gm gmail.Client, _ *slog.Logger) (secrets.Store, gmail.Client, error) {
	return sec, gm, nil
}
