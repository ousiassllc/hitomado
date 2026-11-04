package update

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// NSIS インストーラー（build/windows/nsis/project.nsi）が解釈する引数。
func TestInstallerArgs(t *testing.T) {
	assert.Equal(t, []string{"/S", "/WAIT=60"}, installerArgs(LaunchOptions{}))
	assert.Equal(t, []string{"/S", "/WAIT=60", "/RELAUNCH"}, installerArgs(LaunchOptions{Relaunch: true}))
}
