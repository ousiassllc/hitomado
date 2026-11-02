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
var version = "0.1.0"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err.Error())
		platform.ShowFatal("Hitomado を起動できません", err.Error())
		os.Exit(1)
	}
}
