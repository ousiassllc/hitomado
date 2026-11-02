package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/ousiassllc/hitomado/internal/platform"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// version は -ldflags "-X main.version=..." で上書きする（リリースではタグから注入する）。
// 注入しない dev 実行（wails3 dev）・ローカルビルドの既定値は、VERSION の既定値（build/windows/Taskfile.yml の 0.0.0）と同じにする。
var version = "0.0.0"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err.Error())
		platform.ShowFatal("Hitomado を起動できません", err.Error())
		os.Exit(1)
	}
}
