package appcore_test

import (
	"bytes"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
)

func TestParallelDoRunsAll(t *testing.T) {
	var sum atomic.Int64
	appcore.ParallelDo(slog.New(slog.DiscardHandler), 10, 3, func(i int) { sum.Add(int64(i)) })
	assert.Equal(t, int64(45), sum.Load())
	appcore.ParallelDo(slog.New(slog.DiscardHandler), 0, 3, func(int) { t.Fatal("n=0 では呼ばない") })
}

// fn が panic しても、プロセスは落ちず、ほかの fn は最後まで走り、panic はスタック付きでログに残る。
func TestParallelDoRecoversPanicPerWorker(t *testing.T) {
	var logs bytes.Buffer
	var ran atomic.Int64
	appcore.ParallelDo(slog.New(slog.NewTextHandler(&logs, nil)), 6, 2, func(i int) {
		if i == 2 {
			panic("boom")
		}
		ran.Add(1)
	})
	assert.Equal(t, int64(5), ran.Load())
	assert.Contains(t, logs.String(), "msg=panic")
	assert.Contains(t, logs.String(), "parallel worker 2")
	assert.Contains(t, logs.String(), "stack=")
}
