package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ousiassllc/hitomado/internal/capability"
	"github.com/ousiassllc/hitomado/internal/platform"
)

// singleInstanceOptions は多重起動の防止の設定を返す。OS が持たない（Android。多重起動は OS が管理する）ときは nil。
func singleInstanceOptions(caps capability.Capabilities, mainWindow *windowRef) *application.SingleInstanceOptions {
	if !caps.SingleInstance {
		return nil
	}
	return &application.SingleInstanceOptions{
		UniqueID: "jp.ousias.hitomado",
		OnSecondInstanceLaunch: func(application.SecondInstanceData) {
			if w := mainWindow.get(); w != nil {
				platform.ShowMainWindow(w)
			}
		},
	}
}
