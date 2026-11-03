package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/platform"
	"github.com/ousiassllc/hitomado/internal/update"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// version は -ldflags "-X main.version=..." で上書きする（リリースではタグから注入する）。
// 注入しない dev 実行（wails3 dev）・ローカルビルドの既定値は update.DevVersion（0.0.0）で、更新確認を行わない。
// VERSION の既定値（ルートの Taskfile.yml の includes の vars）と同じにする。
var version = update.DevVersion

func main() {
	if err := runRecovered(); err != nil {
		slog.Error("fatal", "err", err.Error())
		platform.ShowFatal("Hitomado を起動できません", err.Error())
		os.Exit(1)
	}
}

// runRecovered は run の panic をスタックトレースつきでログに残し、通常の起動失敗と同じ経路（エラーダイアログ）へ渡す。
func runRecovered() (err error) {
	defer logging.RecoverTo(slog.Default(), "run", &err)
	return run()
}
