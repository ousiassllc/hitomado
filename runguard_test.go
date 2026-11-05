package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunExclusiveRunsAndReturnsTheResult(t *testing.T) {
	want := errors.New("boom")

	ran, err := runExclusive(func() error { return want })

	assert.True(t, ran)
	assert.ErrorIs(t, err, want)
}

// 実行中の 2 回目（Activity の再生成で nativeInit が main を再び起動したとき）は、何も実行せずに戻る。
func TestRunExclusiveSkipsASecondCallWhileRunning(t *testing.T) {
	var innerRan bool
	var innerCalled bool

	ran, err := runExclusive(func() error {
		innerRan, _ = runExclusive(func() error { innerCalled = true; return errors.New("must not run") })
		return nil
	})

	require.NoError(t, err)
	assert.True(t, ran)
	assert.False(t, innerRan, "実行中の 2 回目は実行しない")
	assert.False(t, innerCalled)
}

// 最初の実行が終わったあとは、再び実行できる（実行中だけを弾く）。
func TestRunExclusiveAllowsACallAfterTheFirstReturned(t *testing.T) {
	_, _ = runExclusive(func() error { return nil })

	ran, _ := runExclusive(func() error { return nil })

	assert.True(t, ran)
}
