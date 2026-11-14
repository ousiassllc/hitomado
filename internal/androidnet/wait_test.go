package androidnet

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scriptedFallback は LookupHost の戻りを順に返す（最後の要素を繰り返す）。
type scriptedFallback struct {
	fakeFallback
	script []error // nil は成功（ips を返す）
	calls  int
}

func (f *scriptedFallback) LookupHost(ctx context.Context, host string) ([]string, error) {
	f.mu.Lock()
	i := min(f.calls, len(f.script)-1)
	f.calls++
	f.mu.Unlock()
	if f.script[i] != nil {
		return nil, f.script[i]
	}
	return f.fakeFallback.LookupHost(ctx, host)
}

func noSleep(slept *[]time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error {
		*slept = append(*slept, d)
		return nil
	}
}

// 背面で OS が通信を止めている（Data Saver・電池の制限）あいだは待ち、アプリが前面へ戻って通るようになったら、引き直して接続する。
// 修正前は最初の no such host で失敗した（#368 の再現。エミュレーターでも Data Saver＋背面で同じエラーになる）。
func TestDialer_WaitsForForegroundThenRecovers(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close(); _ = c2.Close() }()
	blocked := fmt.Errorf("netResolve: %w", ErrBackgroundBlocked)
	fb := &scriptedFallback{
		fakeFallback: fakeFallback{ips: []string{"192.0.2.7"}},
		script:       []error{blocked, blocked, blocked, nil},
	}
	var slept []time.Duration
	var dialed []string
	d := &Dialer{
		Base: func(_ context.Context, _, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			dialed = append(dialed, addr)
			if net.ParseIP(host) == nil {
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			}
			return c1, nil
		},
		Fallback: fb,
		Sleep:    noSleep(&slept),
		Logger:   func() *slog.Logger { return slog.New(slog.DiscardHandler) },
	}

	conn, err := d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	require.NoError(t, err)
	assert.Same(t, c1, conn)
	assert.Equal(t, 4, fb.calls)
	assert.Equal(t, []time.Duration{backgroundWaitInterval, backgroundWaitInterval, backgroundWaitInterval}, slept)
	assert.Equal(t, []string{"auth.openai.com:443", "192.0.2.7:443"}, dialed)
}

// 前面に戻らないまま上限（backgroundWaitAttempts 回）に達したら、元の no such host を返す。
func TestDialer_GivesUpAfterBackgroundWaitLimit(t *testing.T) {
	blocked := fmt.Errorf("netResolve: %w", ErrBackgroundBlocked)
	fb := &scriptedFallback{script: []error{blocked}, fakeFallback: fakeFallback{diag: Diagnostics{Process: "cached", Network: "none", RestrictBackground: "enabled"}}}
	var slept []time.Duration
	logger, logs, logMu := newLogger()
	d := &Dialer{
		Base: func(_ context.Context, _, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		},
		Fallback: fb, Sleep: noSleep(&slept), Resolver: "cgo", Logger: func() *slog.Logger { return logger },
	}

	_, err := d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	var dnsErr *net.DNSError
	require.ErrorAs(t, err, &dnsErr)
	assert.Equal(t, backgroundWaitAttempts, fb.calls)
	assert.Len(t, slept, backgroundWaitAttempts-1)
	logMu.Lock()
	defer logMu.Unlock()
	assert.Contains(t, logs.String(), "process=cached")
	assert.Contains(t, logs.String(), "network=none")
	assert.Contains(t, logs.String(), "restrict_background=enabled")
}

// 待ちの途中で ctx が終わったら、すぐ止める（HTTP クライアントの期限・取り消しに従う）。
func TestDialer_BackgroundWaitStopsWhenContextEnds(t *testing.T) {
	blocked := fmt.Errorf("netResolve: %w", ErrBackgroundBlocked)
	fb := &scriptedFallback{script: []error{blocked}}
	ctx, cancel := context.WithCancel(context.Background())
	d := &Dialer{
		Base: func(_ context.Context, _, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			return nil, &net.DNSError{Err: "no such host", Name: host}
		},
		Fallback: fb,
		Sleep:    func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() },
		Logger:   func() *slog.Logger { return slog.New(slog.DiscardHandler) },
	}
	_, err := d.DialContext(ctx, "tcp", "auth.openai.com:443")
	var dnsErr *net.DNSError
	require.ErrorAs(t, err, &dnsErr)
	assert.Equal(t, 1, fb.calls)
}

// 前面で通信が無い（機内モードなど）ときは待たない。
func TestDialer_ForegroundNoNetworkFailsFast(t *testing.T) {
	fb := &scriptedFallback{script: []error{errors.New("netResolve: document bridge: no_network")}}
	var slept []time.Duration
	d := &Dialer{
		Base: func(_ context.Context, _, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			return nil, &net.DNSError{Err: "no such host", Name: host}
		},
		Fallback: fb, Sleep: noSleep(&slept),
		Logger: func() *slog.Logger { return slog.New(slog.DiscardHandler) },
	}
	_, err := d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	require.Error(t, err)
	assert.Equal(t, 1, fb.calls)
	assert.Empty(t, slept)
}

// 背面で圏外のとき（Java は「OS が通信を止めている」と区別できず ErrBackgroundBlocked を返す）、60 秒ごとの接続が毎回待たないよう、
// 待ちを諦めたあと 1 分は待たずにすぐ失敗する。1 分たてばまた待つ。
func TestDialer_BackgroundWaitCooldownAfterGivingUp(t *testing.T) {
	blocked := fmt.Errorf("netResolve: %w", ErrBackgroundBlocked)
	fb := &fakeFallback{err: blocked}
	var slept []time.Duration
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	d := &Dialer{
		Base: func(_ context.Context, _, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		},
		Fallback: fb, Sleep: noSleep(&slept), Now: func() time.Time { return now },
		Logger: func() *slog.Logger { return slog.New(slog.DiscardHandler) },
	}
	dial := func() {
		t.Helper()
		_, err := d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
		require.Error(t, err)
	}

	dial()
	assert.Len(t, slept, backgroundWaitAttempts-1, "最初は上限まで待つ")
	assert.Len(t, fb.lookups, backgroundWaitAttempts)

	now = now.Add(backgroundWaitCooldown - time.Second)
	dial()
	assert.Len(t, slept, backgroundWaitAttempts-1, "諦めた直後の 1 分は待たない")
	assert.Len(t, fb.lookups, backgroundWaitAttempts+1, "引き直しは 1 回だけ試す")

	now = now.Add(time.Second)
	dial()
	assert.Len(t, slept, 2*(backgroundWaitAttempts-1), "1 分たてばまた待つ")
	assert.Len(t, fb.lookups, 2*backgroundWaitAttempts+1)
}

// 諦めたあとの 1 分のうちに引き直しが通ったら（前面へ戻った）、待ちの抑止を解く。次に止められたときはまた待つ。
func TestDialer_BackgroundWaitCooldownClearedByRecovery(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close(); _ = c2.Close() }()
	blocked := fmt.Errorf("netResolve: %w", ErrBackgroundBlocked)
	fb := &fakeFallback{err: blocked, ips: []string{"192.0.2.7"}}
	var slept []time.Duration
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	d := &Dialer{
		Base: func(_ context.Context, _, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			if net.ParseIP(host) == nil {
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			}
			return c1, nil
		},
		Fallback: fb, Sleep: noSleep(&slept), Now: func() time.Time { return now },
		Logger: func() *slog.Logger { return slog.New(slog.DiscardHandler) },
	}

	_, err := d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	require.Error(t, err)
	require.Len(t, slept, backgroundWaitAttempts-1)

	now = now.Add(10 * time.Second)
	fb.err = nil // 前面へ戻った
	conn, err := d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	require.NoError(t, err)
	assert.Same(t, c1, conn)

	now = now.Add(10 * time.Second)
	fb.err = blocked // また止められた（最初の諦めから 1 分以内）
	_, err = d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	require.Error(t, err)
	assert.Len(t, slept, 2*(backgroundWaitAttempts-1), "回復で抑止が解け、また待つ")
}

// 呼び出し元の ctx が取り消されて止まっただけのときは、失敗の Warn を出さず、1 分の枠も使わない。
func TestDialer_CancelledContextDoesNotLogFailure(t *testing.T) {
	fb := &fakeFallback{err: fmt.Errorf("netResolve: %w", context.Canceled)}
	logger, logs, logMu := newLogger()
	d := &Dialer{
		Base: func(_ context.Context, _, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			return nil, &net.DNSError{Err: "no such host", Name: host}
		},
		Fallback: fb, Logger: func() *slog.Logger { return logger },
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := d.DialContext(ctx, "tcp", "auth.openai.com:443")
	require.Error(t, err)
	logMu.Lock()
	assert.NotContains(t, logs.String(), "name resolution failed")
	logMu.Unlock()
	assert.Zero(t, fb.diagnose)

	// 枠を使っていないので、次の本当の失敗はすぐ記録される。
	fb.err = errors.New("unresolved")
	_, err = d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	require.Error(t, err)
	logMu.Lock()
	defer logMu.Unlock()
	assert.Contains(t, logs.String(), "name resolution failed")
}

// diagCtxFallback は Diagnose に渡された ctx の状態を記録する。
type diagCtxFallback struct {
	fakeFallback
	diagErrSeen   error
	diagHasDeadln bool
}

func (f *diagCtxFallback) Diagnose(ctx context.Context) (Diagnostics, error) {
	f.diagErrSeen = ctx.Err()
	_, f.diagHasDeadln = ctx.Deadline()
	return f.fakeFallback.Diagnose(ctx)
}

// 引き直しが本当に失敗したあとで呼び出し元の ctx が取り消されていても、手がかり（Diagnose）は取り消されない ctx で取る（上限つき）。
func TestDialer_DiagnoseSurvivesCancelledCaller(t *testing.T) {
	fb := &diagCtxFallback{fakeFallback: fakeFallback{err: errors.New("unresolved"), diag: Diagnostics{Process: "cached"}}}
	logger, logs, logMu := newLogger()
	d := &Dialer{
		Base: func(_ context.Context, _, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			return nil, &net.DNSError{Err: "no such host", Name: host}
		},
		Fallback: fb, Logger: func() *slog.Logger { return logger },
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := d.DialContext(ctx, "tcp", "auth.openai.com:443")
	require.Error(t, err)
	assert.NoError(t, fb.diagErrSeen)
	assert.True(t, fb.diagHasDeadln, "diagnoseTimeout の上限がある")
	logMu.Lock()
	defer logMu.Unlock()
	assert.Contains(t, logs.String(), "process=cached")
}
