package oauth_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

// trackerKey は tracker.go の印のキー（非公開）と同じ値。キーの変更は保存済みの印の扱いが変わるため、ここでも固定する。
const trackerKey = "hitomado/oauth_in_progress"

func marked(kv secrets.Store) bool {
	_, err := kv.Get(trackerKey)
	return err == nil
}

func TestTrackerMarkIsSetWhileBrowserIsOpenAndClearedAfterSuccess(t *testing.T) {
	kv := secrets.NewMemory()
	g := newFakeGoogle(t)
	g.tracker = oauth.NewTracker(kv)
	var during bool
	var stored string
	b := &browser{g: g, t: t, behave: func(t *testing.T, p authParams) {
		during = marked(kv)
		stored, _ = kv.Get(trackerKey)
		okCallback(t, p)
	}}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.NoError(t, err)

	assert.True(t, during, "ブラウザに出ている間は印がある")
	assert.False(t, marked(kv), "完了したら印は消える")
	_, perr := time.Parse(time.RFC3339, stored)
	assert.NoError(t, perr, "印は時刻だけ（秘密を含まない）")
	for _, secret := range []string{b.params.State, b.params.Query.Get("code_challenge"), testCode, testClientSecret, testRefresh} {
		assert.NotContains(t, stored, secret)
	}
	assert.False(t, g.tracker.TakeInterrupted(), "正常に終わった認可は中断として扱わない")
}

func TestTrackerMarkIsClearedOnEveryEnd(t *testing.T) {
	cases := map[string]struct {
		behave func(t *testing.T, p authParams)
		openOK bool
		ctx    func() (context.Context, context.CancelFunc)
	}{
		"denied": {behave: func(t *testing.T, p authParams) {
			callback(t, p.Redirect, url.Values{"error": {"access_denied"}, "state": {p.State}})
		}, openOK: true},
		"timeout":    {openOK: true},
		"open error": {openOK: false},
		"cancelled": {openOK: true, ctx: func() (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx, cancel
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			kv := secrets.NewMemory()
			g := newFakeGoogle(t)
			g.tracker = oauth.NewTracker(kv)
			b := &browser{g: g, t: t, behave: c.behave}
			if !c.openOK {
				b.openErr = assert.AnError
			}
			ctx := context.Background()
			if c.ctx != nil {
				var cancel context.CancelFunc
				ctx, cancel = c.ctx()
				defer cancel()
			}

			_, err := g.flow(b.open, 50*time.Millisecond).Authorize(ctx, cfg)

			require.Error(t, err)
			assert.False(t, marked(kv))
			assert.False(t, g.tracker.TakeInterrupted())
		})
	}
}

// プロセスがブラウザに出ている間に止められたことを、次の起動（同じ資格情報ストアを読む新しい Tracker）が 1 回だけ検出する。
func TestTrackerDetectsProcessKilledWhileBrowserIsOpen(t *testing.T) {
	kv := secrets.NewMemory()
	g := newFakeGoogle(t)
	g.tracker = oauth.NewTracker(kv)
	opened := make(chan struct{})
	b := &browser{g: g, t: t, behave: func(*testing.T, authParams) { close(opened) }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := g.flow(b.open, time.Minute).Authorize(ctx, cfg)
		done <- err
	}()
	<-opened
	require.True(t, marked(kv))

	// ここでプロセスが死ぬ（End は呼ばれない）。次の起動:
	next := oauth.NewTracker(kv)
	assert.True(t, next.TakeInterrupted(), "中断を検出して『ログインはやり直し』を案内できる")
	assert.False(t, next.TakeInterrupted(), "案内は 1 回だけ")
	assert.False(t, marked(kv), "印は消える（ログイン中の状態を残さない）")

	cancel() // 死んだはずの古い認可のゴルーチンを片づける
	require.ErrorIs(t, <-done, oauth.ErrCancelled)
}

// 中断の案内の前に新しい認可を始めても、前回の中断は失われず、新しい認可は普通に完了する。
func TestTrackerNewAuthorizeAfterInterruptedStillCompletes(t *testing.T) {
	kv := secrets.NewMemory()
	require.NoError(t, kv.Set(trackerKey, "2026-11-03T00:00:00Z")) // 前回の起動が残した印

	g := newFakeGoogle(t)
	g.tracker = oauth.NewTracker(kv)
	b := &browser{g: g, t: t, behave: okCallback}
	res, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.NoError(t, err)

	assert.Equal(t, testRefresh, res.RefreshToken)
	assert.False(t, marked(kv))
	assert.True(t, g.tracker.TakeInterrupted(), "前回の中断も案内する")
	assert.False(t, g.tracker.TakeInterrupted())
}

func TestTrackerNilIsNoop(t *testing.T) {
	var tr *oauth.Tracker
	tr.Begin()
	tr.End()
	assert.False(t, tr.TakeInterrupted())

	g := newFakeGoogle(t) // tracker なしの Flow（Windows）も従来どおり完了する
	b := &browser{g: g, t: t, behave: okCallback}
	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.NoError(t, err)
}

func TestTrackerStoreFailureDoesNotBreakAuthorize(t *testing.T) {
	g := newFakeGoogle(t)
	g.tracker = oauth.NewTracker(failingStore{})
	b := &browser{g: g, t: t, behave: okCallback}
	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.NoError(t, err)
	assert.False(t, g.tracker.TakeInterrupted())
}

type failingStore struct{}

func (failingStore) Get(string) (string, error) { return "", assert.AnError }
func (failingStore) Set(string, string) error   { return assert.AnError }
func (failingStore) Delete(string) error        { return assert.AnError }
