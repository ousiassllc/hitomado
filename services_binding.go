package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/capability"
)

// bindingServices はフロントへ公開する（バインディングの）サービスを返す。
// MCP の機能が無い OS（Android）では MCPService を登録しない（フロントから MCP の設定・トークンを操作できない）。
// 型（バインディング）は OS によらず同じで、登録するかどうかだけが変わる。
func bindingServices(svc *app.Services, caps capability.Capabilities) []application.Service {
	out := []application.Service{
		application.NewService(svc.Account), application.NewService(svc.Inbox),
		application.NewService(svc.Thread), application.NewService(svc.Compose),
		application.NewService(svc.AI), application.NewService(svc.Agent), application.NewService(svc.App),
		application.NewService(svc.Log), application.NewService(capability.NewPlatformService(caps)),
	}
	if caps.SupportsMCP {
		out = append(out, application.NewService(svc.MCP))
	}
	return out
}
