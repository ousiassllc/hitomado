package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ousiassllc/hitomado/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a := application.New(application.Options{
		Name: "Hitomado",
		Services: []application.Service{
			application.NewService(&app.AccountService{}),
			application.NewService(&app.InboxService{}),
			application.NewService(&app.ThreadService{}),
			application.NewService(&app.ComposeService{}),
			application.NewService(&app.AIService{}),
			application.NewService(&app.AppService{}),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
	})
	a.Window.NewWithOptions(application.WebviewWindowOptions{Title: "Hitomado", Width: 1280, Height: 800, URL: "/"})
	if err := a.Run(); err != nil {
		log.Fatal(err)
	}
}
