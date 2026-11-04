package apptests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apptest"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/updatecore"
	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/update"
)

// tickerFor は手動で刻める ticker を Deps に差し込む。
func tickerFor(e *apptest.Env) chan time.Time {
	ch := make(chan time.Time)
	e.Deps.UpdateTicker = func() (<-chan time.Time, func()) { return ch, func() {} }
	return ch
}

func runLoop(t *testing.T, e *apptest.Env) (cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	d := make(chan struct{})
	go func() { defer close(d); updatecore.NewLoop((*deps.Deps)(e.Deps)).Run(ctx) }()
	t.Cleanup(func() { cancel(); <-d })
	return cancel, d
}

func waitEvents(t *testing.T, e *apptest.Env, n int) []apptest.Emitted {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	got := e.Events.Wait(ctx, events.UpdateAvailable, n)
	require.GreaterOrEqual(t, len(got), n, "update:available が %d 回発行されること", n)
	return got
}

func TestUpdateLoopEmitsAtStartAndOnTick(t *testing.T) {
	_, e := newAppSvc(t)
	tick := tickerFor(e)
	e.Update.Release = update.Release{Version: "0.3.0", URL: "https://example.com/r"}
	cancel, done := runLoop(t, e)

	got := waitEvents(t, e, 1)
	assert.Equal(t, events.UpdateAvailablePayload{Version: "0.3.0", URL: "https://example.com/r"}, got[0].Payload)

	tick <- time.Now() // ループが次の確認待ちに入っていれば受け取る
	waitEvents(t, e, 2)
	assert.Equal(t, 2, e.Update.CallCount())

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("UpdateLoop.Run が ctx キャンセルで終了しない")
	}
}

func TestUpdateLoopSkipsWhenDisabled(t *testing.T) {
	svc, e := newAppSvc(t)
	tick := tickerFor(e)
	require.NoError(t, svc.SetUpdateCheckEnabled(t.Context(), false))
	e.Update.Release = update.Release{Version: "9.0.0"}
	runLoop(t, e)

	tick <- time.Now() // 受け取られた時点で、起動時の確認は完了している
	tick <- time.Now() // 同様に 2 回目の確認が完了している
	assert.Zero(t, e.Update.CallCount(), "無効なら通信しない")
	assert.Empty(t, e.Events.Named(events.UpdateAvailable))
}

func TestUpdateLoopSkipsDevBuild(t *testing.T) {
	_, e := newAppSvc(t)
	tick := tickerFor(e)
	e.Deps.Version = update.DevVersion
	e.Update.Release = update.Release{Version: "9.0.0", URL: "u"}
	runLoop(t, e)

	tick <- time.Now() // 受け取られた時点で、起動時の確認は完了している
	tick <- time.Now() // 同様に 2 回目の確認が完了している
	assert.Zero(t, e.Update.CallCount(), "dev ビルドは通信しない")
	assert.Empty(t, e.Events.Named(events.UpdateAvailable))
}

func TestUpdateLoopDoesNotEmitForSameVersion(t *testing.T) {
	_, e := newAppSvc(t)
	tick := tickerFor(e)
	e.Update.Release = update.Release{Version: "0.1.0", URL: "u"}
	runLoop(t, e)

	tick <- time.Now()
	tick <- time.Now()
	assert.GreaterOrEqual(t, e.Update.CallCount(), 2, "有効なら確認する")
	assert.Empty(t, e.Events.Named(events.UpdateAvailable), "同じ版では発行しない")
}

func TestUpdateLoopSurvivesCheckFailure(t *testing.T) {
	_, e := newAppSvc(t)
	tick := tickerFor(e)
	e.Update.Err = gmail.ErrOffline
	runLoop(t, e)
	tick <- time.Now() // 失敗した起動時確認の後でも次の周期を受け取る
	e.Update.Set(update.Release{Version: "1.0.0", URL: "u"}, nil)
	tick <- time.Now()
	waitEvents(t, e, 1)
}
