package capability_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/capability"
)

func TestDesktopEnablesEverything(t *testing.T) {
	assert.Equal(t, capability.Capabilities{
		OS: "windows", SupportsMCP: true, SupportsTray: true, SupportsAutoUpdate: true, SupportsWindowState: true, SingleInstance: true,
	}, capability.Desktop("windows"))
}

func TestAndroidDisablesEverything(t *testing.T) {
	assert.Equal(t, capability.Capabilities{OS: "android"}, capability.Android())
}

func TestZeroValueDisablesEverything(t *testing.T) {
	c := capability.Capabilities{}
	assert.False(t, c.SupportsMCP || c.SupportsTray || c.SupportsAutoUpdate || c.SupportsWindowState || c.SingleInstance)
}

// このテストを実行するホスト（Android ではない）の Current はデスクトップの機能になる。
func TestCurrentOnHostIsDesktop(t *testing.T) {
	assert.Equal(t, capability.Desktop(capability.Current().OS), capability.Current())
}

func TestPlatformServiceReturnsGivenCapabilities(t *testing.T) {
	assert.Equal(t, capability.Android(), capability.NewPlatformService(capability.Android()).GetCapabilities())
	assert.Equal(t, capability.Desktop("windows"), capability.NewPlatformService(capability.Desktop("windows")).GetCapabilities())
}

// フロントの型（frontend/bindings）と store が読む JSON のキー。
func TestCapabilitiesJSONKeys(t *testing.T) {
	b, err := json.Marshal(capability.Android())
	require.NoError(t, err)
	assert.JSONEq(t, `{"os":"android","supportsMcp":false,"supportsTray":false,"supportsAutoUpdate":false,"supportsWindowState":false,"singleInstance":false}`, string(b))
}
