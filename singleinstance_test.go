package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/capability"
)

func TestSingleInstanceOptionsFollowCapability(t *testing.T) {
	opts := singleInstanceOptions(capability.Desktop("windows"), &windowRef{})
	if assert.NotNil(t, opts) {
		assert.Equal(t, "jp.ousias.hitomado", opts.UniqueID)
		assert.NotNil(t, opts.OnSecondInstanceLaunch)
	}
	assert.Nil(t, singleInstanceOptions(capability.Android(), &windowRef{}), "Android は多重起動の防止を設定しない")
}
