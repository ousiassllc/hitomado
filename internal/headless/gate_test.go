package headless

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gateWait = 5 * time.Second

func ok(context.Context) (Result, error) { return Result{Accounts: []AccountResult{{ID: "a"}}}, nil }

func TestGateRunsHeadlessWhenAppIsNotRunning(t *testing.T) {
	var g Gate
	res, err := g.Run(context.Background(), ok)
	require.NoError(t, err)
	assert.False(t, res.AppRunning)
	assert.Len(t, res.Accounts, 1)
}

func TestGateRejectsConcurrentHeadlessRuns(t *testing.T) {
	var g Gate
	started, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := g.Run(context.Background(), func(context.Context) (Result, error) {
			close(started)
			<-release
			return Result{}, nil
		})
		finished <- err
	}()
	<-started

	_, err := g.Run(context.Background(), ok)
	assert.ErrorIs(t, err, ErrBusy)

	close(release)
	require.NoError(t, <-finished)
	_, err = g.Run(context.Background(), ok)
	assert.NoError(t, err, "終わったら次を実行できる")
}

func TestGateDelegatesToRunningApp(t *testing.T) {
	var g Gate
	g.AppStarting()

	// アプリ本体の組み立て中（窓口が未設定）は何もしないで成功。
	res, err := g.Run(context.Background(), func(context.Context) (Result, error) {
		t.Fatal("アプリ本体が動いているときにヘッドレス同期を始めない")
		return Result{}, nil
	})
	require.NoError(t, err)
	assert.True(t, res.AppRunning)
	assert.Empty(t, res.NewMail)

	called := 0
	g.SetAppRefresh(func(context.Context) ([]AccountResult, error) {
		called++
		return []AccountResult{{ID: "a", Status: StatusOK}}, nil
	})
	res, err = g.Run(context.Background(), ok)
	require.NoError(t, err)
	assert.True(t, res.AppRunning)
	assert.Equal(t, 1, called)
	assert.Equal(t, []AccountResult{{ID: "a", Status: StatusOK}}, res.Accounts)

	g.SetAppRefresh(func(context.Context) ([]AccountResult, error) { return nil, errors.New("boom") })
	_, err = g.Run(context.Background(), ok)
	assert.Error(t, err)
}

func TestGateAppStartingCancelsRunningHeadlessAndWaits(t *testing.T) {
	var g Gate
	started := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = g.Run(context.Background(), func(ctx context.Context) (Result, error) {
			close(started)
			<-ctx.Done()
			return Result{}, ctx.Err()
		})
	}()
	<-started

	returned := make(chan struct{})
	go func() { g.AppStarting(); close(returned) }()
	select {
	case <-returned:
	case <-time.After(gateWait):
		t.Fatal("AppStarting が戻らない")
	}
	select {
	case <-finished:
	default:
		t.Fatal("AppStarting はヘッドレス同期の終了を待ってから戻る（DB を同時に開かない）")
	}
}

func TestGateCancelStopsRunningHeadless(t *testing.T) {
	var g Gate
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := g.Run(context.Background(), func(ctx context.Context) (Result, error) {
			close(started)
			<-ctx.Done()
			return Result{}, ctx.Err()
		})
		result <- err
	}()
	<-started
	g.Cancel()
	select {
	case err := <-result:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(gateWait):
		t.Fatal("Cancel で止まらない")
	}
	g.Cancel() // 何も走っていなくても安全
}

func TestGateAppStoppedReturnsToHeadless(t *testing.T) {
	var g Gate
	g.AppStarting()
	g.SetAppRefresh(func(context.Context) ([]AccountResult, error) { return nil, nil })
	g.AppStopped()
	res, err := g.Run(context.Background(), ok)
	require.NoError(t, err)
	assert.False(t, res.AppRunning)
}
