package androidnet

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResolverBase は、IP リテラルだけに接続でき、名前は `lookup <host>: no such host`（*net.DNSError。#368 の実機と同じ形）で
// 失敗する Base。標準の解決器（getaddrinfo）が壊れた端末の再現。target はテスト用サーバーのアドレスで、IP への接続はそこへ向ける。
func fakeResolverBase(target string, dialed *[]string, mu *sync.Mutex) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		mu.Lock()
		*dialed = append(*dialed, addr)
		mu.Unlock()
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if net.ParseIP(host) == nil {
			return nil, &net.OpError{Op: "dial", Net: network, Err: &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}}
		}
		var d net.Dialer
		return d.DialContext(ctx, network, target)
	}
}

type fakeFallback struct {
	mu       sync.Mutex
	ips      []string
	err      error
	diag     Diagnostics
	diagErr  error
	lookups  []string
	diagnose int
}

func (f *fakeFallback) LookupHost(_ context.Context, host string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups = append(f.lookups, host)
	return f.ips, f.err
}

func (f *fakeFallback) Diagnose(context.Context) (Diagnostics, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.diagnose++
	return f.diag, f.diagErr
}

func newLogger() (*slog.Logger, *bytes.Buffer, *sync.Mutex) {
	buf := &bytes.Buffer{}
	mu := &sync.Mutex{}
	return slog.New(slog.NewTextHandler(&lockedWriter{buf: buf, mu: mu}, nil)), buf, mu
}

type lockedWriter struct {
	buf *bytes.Buffer
	mu  *sync.Mutex
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func testServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	t.Cleanup(srv.Close)
	return srv, srv.Listener.Addr().String()
}

// 標準の解決が名前を引けなくても、Android の API の引き直しで HTTP 通信が通る（修正前は no such host で失敗した）。
func TestDialer_RecoversThroughFallbackOverHTTP(t *testing.T) {
	_, target := testServer(t)
	var dialed []string
	var mu sync.Mutex
	fb := &fakeFallback{ips: []string{"127.0.0.1"}}
	logger, logs, logMu := newLogger()
	d := &Dialer{Base: fakeResolverBase(target, &dialed, &mu), Fallback: fb, Resolver: "cgo", Logger: func() *slog.Logger { return logger }}

	// 修正前（標準の解決だけ）は #368 と同じ no such host で失敗する。
	plain := &http.Transport{DialContext: d.Base}
	req, err := http.NewRequest(http.MethodGet, "http://auth.example.test:80/", nil)
	require.NoError(t, err)
	_, err = (&http.Client{Transport: plain, Timeout: 10 * time.Second}).Do(req) //nolint:bodyclose // 失敗する想定（応答は無い）
	var dnsErr *net.DNSError
	require.ErrorAs(t, err, &dnsErr)
	assert.True(t, dnsErr.IsNotFound)

	tr := &http.Transport{}
	Install(tr, d)
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get("http://auth.example.test:80/token?code=secret-code")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", string(body))

	assert.Equal(t, []string{"auth.example.test"}, fb.lookups)
	logMu.Lock()
	defer logMu.Unlock()
	assert.Contains(t, logs.String(), "name resolution recovered")
	assert.Contains(t, logs.String(), "host=auth.example.test")
	assert.NotContains(t, logs.String(), "secret-code", "URL・クエリはログに載せない")
}

// Android の API でも引けないときは元のエラー（*net.DNSError）をそのまま返し、手がかりつきで Warn に残す。
func TestDialer_LogsDiagnosticsWhenBothResolutionsFail(t *testing.T) {
	_, target := testServer(t)
	var dialed []string
	var mu sync.Mutex
	fb := &fakeFallback{
		err:  errors.New("netResolve: android bridge unavailable"),
		diag: Diagnostics{Process: "cached", Network: "wifi", Validated: true, PrivateDNS: "strict", DNSServers: []string{"192.0.2.53", "not-an-ip"}, RestrictBackground: "enabled", BackgroundRestricted: true, StandbyBucket: "rare"},
	}
	logger, logs, logMu := newLogger()
	d := &Dialer{Base: fakeResolverBase(target, &dialed, &mu), Fallback: fb, Resolver: "cgo", Logger: func() *slog.Logger { return logger }}

	_, err := d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	var dnsErr *net.DNSError
	require.ErrorAs(t, err, &dnsErr)
	assert.Equal(t, "auth.openai.com", dnsErr.Name)

	logMu.Lock()
	defer logMu.Unlock()
	out := logs.String()
	assert.Contains(t, out, "level=WARN")
	assert.Contains(t, out, `msg="net: name resolution failed"`)
	for _, want := range []string{
		"host=auth.openai.com", "resolver=cgo", "no such host", "fallback_err=", "android bridge unavailable",
		"process=cached", "network=wifi", "private_dns=strict", "dns_servers=192.0.2.53", "restrict_background=enabled",
		"background_restricted=true", "standby_bucket=rare",
	} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "not-an-ip", "IP として読めない値は載せない")
}

func TestDialer_LogsDiagnoseFailureInsteadOfDroppingTheLog(t *testing.T) {
	var dialed []string
	var mu sync.Mutex
	fb := &fakeFallback{err: errors.New("unresolved"), diagErr: errors.New("netDiagnose: android bridge unavailable")}
	logger, logs, logMu := newLogger()
	d := &Dialer{Base: fakeResolverBase("127.0.0.1:1", &dialed, &mu), Fallback: fb, Resolver: "go", Logger: func() *slog.Logger { return logger }}

	_, err := d.DialContext(context.Background(), "tcp", "oauth2.googleapis.com:443")
	require.Error(t, err)
	logMu.Lock()
	defer logMu.Unlock()
	assert.Contains(t, logs.String(), "diagnose_err=")
	assert.Contains(t, logs.String(), "resolver=go")
}

// 名前解決以外の失敗（接続拒否など）は、Android の API を呼ばずそのまま返す。
func TestDialer_NonDNSErrorsPassThrough(t *testing.T) {
	fb := &fakeFallback{ips: []string{"127.0.0.1"}}
	refused := errors.New("connection refused")
	d := &Dialer{
		Base:     func(context.Context, string, string) (net.Conn, error) { return nil, refused },
		Fallback: fb,
	}
	_, err := d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	assert.ErrorIs(t, err, refused)
	assert.Empty(t, fb.lookups)
	assert.Zero(t, fb.diagnose)
}

// 標準の解決が通るなら、何も変えない（Android の API を呼ばない）。
func TestDialer_SuccessDoesNotUseFallback(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close(); _ = c2.Close() }()
	fb := &fakeFallback{ips: []string{"127.0.0.1"}}
	d := &Dialer{
		Base:     func(context.Context, string, string) (net.Conn, error) { return c1, nil },
		Fallback: fb,
	}
	conn, err := d.DialContext(context.Background(), "tcp", "auth.openai.com:443")
	require.NoError(t, err)
	assert.Same(t, c1, conn)
	assert.Empty(t, fb.lookups)
}

// 引き直したアドレスのうち、接続できるものまで順に試す。ファミリーの違うもの・IP でないものは試さない。
func TestDialer_TriesAddressesInOrderAndFiltersFamily(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close(); _ = c2.Close() }()
	var tried []string
	d := &Dialer{
		Base: func(_ context.Context, network, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			if net.ParseIP(host) == nil {
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			}
			tried = append(tried, network+" "+addr)
			if host == "192.0.2.2" {
				return c1, nil
			}
			return nil, errors.New("unreachable")
		},
		Fallback: &fakeFallback{ips: []string{"2001:db8::1", "junk", "192.0.2.1", "192.0.2.2"}},
	}
	conn, err := d.DialContext(context.Background(), "tcp4", "auth.openai.com:443")
	require.NoError(t, err)
	assert.Same(t, c1, conn)
	assert.Equal(t, []string{"tcp4 192.0.2.1:443", "tcp4 192.0.2.2:443"}, tried)
}

func TestDialer_StopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tried := 0
	d := &Dialer{
		Base: func(_ context.Context, _, addr string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			if net.ParseIP(host) == nil {
				return nil, &net.DNSError{Err: "no such host", Name: host}
			}
			tried++
			cancel()
			return nil, errors.New("unreachable")
		},
		Fallback: &fakeFallback{ips: []string{"192.0.2.1", "192.0.2.2"}},
		Logger:   func() *slog.Logger { return slog.New(slog.DiscardHandler) },
	}
	_, err := d.DialContext(ctx, "tcp", "auth.openai.com:443")
	var dnsErr *net.DNSError
	assert.ErrorAs(t, err, &dnsErr)
	assert.Equal(t, 1, tried, "取り消し後は残りのアドレスを試さない")
}

// 同期が失敗し続けても、同じホストのログを 1 分に 1 回までにする。
func TestDialer_ThrottlesRepeatedLogs(t *testing.T) {
	var dialed []string
	var mu sync.Mutex
	fb := &fakeFallback{err: errors.New("unresolved")}
	logger, logs, logMu := newLogger()
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	d := &Dialer{
		Base: fakeResolverBase("127.0.0.1:1", &dialed, &mu), Fallback: fb,
		Logger: func() *slog.Logger { return logger }, Now: func() time.Time { return now },
	}
	dial := func(host string) { _, _ = d.DialContext(context.Background(), "tcp", host+":443") }
	count := func() int {
		logMu.Lock()
		defer logMu.Unlock()
		return strings.Count(logs.String(), "name resolution failed")
	}

	dial("auth.openai.com")
	dial("auth.openai.com")
	assert.Equal(t, 1, count())
	dial("oauth2.googleapis.com")
	assert.Equal(t, 2, count(), "別のホストは別に数える")
	now = now.Add(logInterval)
	dial("auth.openai.com")
	assert.Equal(t, 3, count())
}

func TestInstall_ReplacesDialContextOnly(t *testing.T) {
	d := &Dialer{Base: func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("x") }}
	tr := &http.Transport{Proxy: http.ProxyFromEnvironment}
	Install(tr, d)
	assert.NotNil(t, tr.DialContext)
	assert.NotNil(t, tr.Proxy)
	Install(nil, d) // パニックしない
}
