package backoff

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelayCapsAtSixtySeconds(t *testing.T) {
	assert.Equal(t, time.Second, Delay(0))
	assert.Equal(t, 32*time.Second, Delay(5))
	assert.Equal(t, 60*time.Second, Delay(6))
	assert.Equal(t, 60*time.Second, Delay(80))
}

func TestFullJitterStaysWithinBound(t *testing.T) {
	for range 200 {
		d := FullJitter(4 * time.Second)
		assert.GreaterOrEqual(t, d, time.Duration(0))
		assert.LessOrEqual(t, d, 4*time.Second)
	}
}

func TestSleepStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, Sleep(ctx, time.Hour), context.Canceled)
	require.NoError(t, Sleep(context.Background(), time.Millisecond))
}
