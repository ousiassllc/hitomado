package siwctests

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/siwc/siwctest"
)

// lockWait は「待たされない」ことの確認の上限。ロックが全アカウントで 1 本なら、この間に終わらない。
const lockWait = 5 * time.Second

// gateRevoke は clientID の失効の通信を、release が閉じるまで止める。通信が始まったら started が閉じる。
func gateRevoke(t *testing.T, e *siwctest.Env, clientID string) (started <-chan struct{}, release func()) {
	t.Helper()
	startedCh, releaseCh := make(chan struct{}), make(chan struct{})
	var once, relOnce sync.Once
	e.F.Mu.Lock()
	e.F.RevokeHook = func(id string) {
		if id == clientID {
			once.Do(func() { close(startedCh) })
			<-releaseCh
		}
	}
	e.F.Mu.Unlock()
	release = func() { relOnce.Do(func() { close(releaseCh) }) }
	t.Cleanup(release)
	return startedCh, release
}

// within は ch から lockWait 以内に値を受け取る。
func within[T any](t *testing.T, ch <-chan T, msg string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(lockWait):
		t.Fatal(msg)
		panic("unreachable")
	}
}

// アカウント A のログアウト（失効の通信中）に、アカウント B のアクセストークンの更新・ログアウトが待たされない。
func TestSignOutOfOneAccountDoesNotBlockAnotherAccount(t *testing.T) {
	e := twoAccounts(t, nil, nil) // 使用中は secondClient（B）
	started, release := gateRevoke(t, e, siwctest.IssuedClient)
	signedOut := make(chan error, 1)
	go func() {
		_, err := e.M.SignOut(t.Context(), siwctest.IssuedClient) // A のログアウト
		signedOut <- err
	}()
	within(t, started, "A の失効の通信が始まらない")

	e.Clk.Advance(2 * time.Hour) // B は更新が要る（通信を伴う）
	tok := make(chan string, 1)
	go func() {
		s, err := e.M.AccessToken(t.Context())
		assert.NoError(t, err)
		tok <- s
	}()
	assert.Equal(t, secondAccess+"-refreshed-1", within(t, tok, "A のログアウトの間、B の AccessToken が待たされた"))

	out := make(chan error, 1)
	go func() {
		_, err := e.M.SignOut(t.Context(), secondClient)
		out <- err
	}()
	require.NoError(t, within(t, out, "A のログアウトの間、B のログアウトが待たされた"))

	assert.Empty(t, signedOut, "A のログアウトは失効の通信が終わるまで続く")
	release()
	require.NoError(t, within(t, signedOut, "A のログアウトが終わらない"))
}

// 同じアカウントのログアウト中の更新は、ログアウトが終わるまで待つ（回転するトークンを競合させない）。
func TestRefreshOfTheSameAccountWaitsForItsSignOut(t *testing.T) {
	e := twoAccounts(t, nil, nil) // 使用中は B
	started, release := gateRevoke(t, e, secondClient)
	go func() { _, _ = e.M.SignOut(t.Context(), secondClient) }()
	within(t, started, "失効の通信が始まらない")

	e.Clk.Advance(2 * time.Hour)
	res := make(chan error, 1)
	go func() {
		_, err := e.M.AccessToken(t.Context())
		res <- err
	}()
	time.Sleep(100 * time.Millisecond) // 待たされていること（終わらないこと）の確認。短くても誤って失敗はしない
	assert.Empty(t, res, "ログアウト中の同じアカウントの更新が待たずに終わった")
	release()
	assert.ErrorIs(t, within(t, res, "ログアウトが終わっても更新が終わらない"), siwc.ErrNotSignedIn)
	assert.Len(t, e.F.TokenFormsCopy(), 2, "ログアウト済みのアカウントは更新しない（ログイン 2 回分のみ）")
}
