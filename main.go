package main

import (
	"embed"
	"errors"
	"log"
	"log/slog"
	"os"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// version は -ldflags "-X main.version=..." で上書きする。
var version = "0.1.0"

var errNotReady = errors.New("app not ready")

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err.Error())
		log.New(os.Stderr, "", 0).Print("Hitomado: ", err)
		os.Exit(1)
	}
}
