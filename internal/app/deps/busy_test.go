package deps_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/app/deps"
)

func TestBusy(t *testing.T) {
	var b deps.Busy
	assert.False(t, b.Active())
	end1 := b.Begin()
	end2 := b.Begin()
	assert.True(t, b.Active())
	end1()
	assert.True(t, b.Active(), "1 つでも進行中なら進行中")
	end2()
	assert.False(t, b.Active())
}

func TestNilBusyIsAlwaysIdle(t *testing.T) {
	var b *deps.Busy
	end := b.Begin()
	assert.False(t, b.Active())
	end()
}
