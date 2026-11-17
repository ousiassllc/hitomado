package cooldown_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/gmail/cooldown"
)

func TestCooldownKeepsTheLongestWait(t *testing.T) {
	var c cooldown.Cooldown
	assert.Zero(t, c.Remaining())
	c.Extend(time.Hour)
	c.Extend(time.Minute) // 短い待ちで縮めない
	assert.Greater(t, c.Remaining(), 59*time.Minute)
}
