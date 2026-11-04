package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/app"
	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/capability"
)

func TestBindingServicesFollowCapability(t *testing.T) {
	svc := app.NewServices(apptest.NewDeps(t))
	instances := func(caps capability.Capabilities) []any {
		var out []any
		for _, s := range bindingServices(svc, caps) {
			out = append(out, s.Instance())
		}
		return out
	}

	hasPlatform := func(instances []any) bool {
		for _, i := range instances {
			if _, ok := i.(*capability.PlatformService); ok {
				return true
			}
		}
		return false
	}

	desktop := instances(capability.Desktop("windows"))
	assert.Contains(t, desktop, any(svc.MCP))
	assert.True(t, hasPlatform(desktop))

	android := instances(capability.Android())
	assert.NotContains(t, android, any(svc.MCP), "Android では MCPService を登録しない")
	assert.True(t, hasPlatform(android), "PlatformService は全 OS で登録する")
	assert.Contains(t, android, any(svc.App))
}
