package platform

import (
	"testing"
	"time"
)

const testBound = 20 * time.Millisecond

func TestWaitBoundedReturnsTrueWhenFnFinishesInTime(t *testing.T) {
	t.Parallel()
	ran := false
	if !WaitBounded(func() { ran = true }, time.Minute) {
		t.Fatal("WaitBounded = false, want true for fast fn")
	}
	if !ran {
		t.Fatal("fn did not run")
	}
}

func TestWaitBoundedGivesUpOnBlockedFn(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	finished := make(chan struct{})
	result := make(chan bool, 1)
	go func() {
		result <- WaitBounded(func() { <-release; close(finished) }, testBound)
	}()

	select {
	case ok := <-result:
		if ok {
			t.Fatal("WaitBounded = true, want false for blocked fn")
		}
	case <-time.After(10 * time.Second): // 失敗時にハングさせないための安全網（通常は testBound で戻る）
		close(release)
		t.Fatal("WaitBounded did not return within its bound")
	}

	// 放棄された呼び出しは後から終わる（パニックも待ち残しもない）。
	close(release)
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("abandoned fn never finished")
	}
}
