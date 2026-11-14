// Package androidnet は Android で、アプリ本体（Go）の HTTPS 通信が名前解決に失敗したときの救済と記録を行う（#368）。
//
// Go の net パッケージは Android（GOOS=android・cgo）では bionic の getaddrinfo を使う。getaddrinfo が失敗すると
// `lookup auth.openai.com: no such host`（*net.DNSError。Server が空）になり、Go の側からは原因が分からない。
// このパッケージは次の 2 つだけを行う。いずれも Android 専用で、Windows などの挙動は変えない（install_android.go だけが有効にする）。
//   - 接続（http.DefaultTransport の DialContext）で名前解決が失敗したら、Android の API（ConnectivityManager の有効なネットワークの
//     Network.getAllByName）で引き直して、得た IP へ接続する。OS 標準の解決が通るときは何も変えない。
//     アプリが背面で、OS が通信を止めている（Data Saver・電池の「制限」など。有効なネットワークが無く、getaddrinfo は
//     no such host になる）ときは、利用者がアプリへ戻るのを短い間だけ待って引き直す（ErrBackgroundBlocked）。
//   - 失敗したとき、どの解決器・どの DNS の状態で失敗したかをログに残す（トークン・URL・メールは載せない）。
//
// 公開 DNS（8.8.8.8 など）のアドレスは決め打ちしない。端末の Private DNS・VPN・社内 DNS を迂回し、DNS の問い合わせ先を勝手に
// 変えるプライバシー上の後退になるため、救済は常に端末が使う DNS の設定（Network）を通す。
package androidnet

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// ErrBackgroundBlocked は Fallback.LookupHost の失敗のうち「アプリが背面で、有効なネットワークが無い」もの。OS が背面のアプリの通信を
// 止めている（Data Saver・電池の制限など）状態で、アプリが前面に戻れば通るようになる。待って引き直してよい。
var ErrBackgroundBlocked = errors.New("network unavailable while the app is in the background")

// Fallback は OS 標準の名前解決が失敗したときに使う、Android の API による名前解決と状態の取得。
type Fallback interface {
	// LookupHost は有効なネットワークで host を引き直す（Network.getAllByName）。
	LookupHost(ctx context.Context, host string) ([]string, error)
	// Diagnose は失敗の手がかり（アプリの前面・背面、ネットワークの種類、DNS の状態）を返す。取れなければエラー。
	Diagnose(ctx context.Context) (Diagnostics, error)
}

const (
	// attemptTimeout は 1 アドレスへの接続の上限（引き直した IP を順に試すときの、1 件あたり）。
	attemptTimeout = 10 * time.Second
	// diagnoseTimeout は失敗時の状態取得（JNI）の待ちの上限。
	diagnoseTimeout = 3 * time.Second
	// logInterval は同じホストの同じ種類のログを出す最短の間隔。同期が 60 秒ごとに失敗し続けても、ログを埋めない。
	logInterval = time.Minute
	// backgroundWaitInterval・backgroundWaitAttempts は ErrBackgroundBlocked のときに、アプリが前面へ戻るのを待つ間隔と回数
	// （引き直しを最大 20 回試し、その間の待ちは 19 回なので合計で約 19 秒。トークン交換などの HTTP クライアントの期限は 30 秒なので、
	// 残りで TLS と要求を行える）。前面のアプリで通信が無い（機内モードなど）ときは待たず、従来どおりすぐ失敗する。
	backgroundWaitInterval = time.Second
	backgroundWaitAttempts = 20
	// backgroundWaitCooldown は、前面へ戻らないまま待ちを諦めたあと、次に待ちを始めるまでの間隔。背面で本当に圏外のとき
	// （Java は「OS が背面の通信を止めている」と「圏外」を区別できず、どちらも ErrBackgroundBlocked を返す）、60 秒ごとの同期が
	// 毎回約 19 秒待つのを防ぐ。この間の接続は、引き直しを 1 回だけ試して従来どおりすぐ失敗する。
	backgroundWaitCooldown = logInterval
)

// Dialer は http.Transport の DialContext に差し込む、名前解決の救済つきの接続。
type Dialer struct {
	// Base は OS 標準の解決を使う接続（net.Dialer.DialContext）。
	Base func(ctx context.Context, network, addr string) (net.Conn, error)
	// Fallback は Base が名前解決で失敗したときの救済。
	Fallback Fallback
	// Logger は記録先。呼び出しごとに引く（slog.SetDefault が後から差し替わるため）。nil なら slog.Default。
	Logger func() *slog.Logger
	// Resolver は標準の解決器の種類（cgo か go）。ログに載せる。
	Resolver string
	// Now は時刻（ログの間引き・待ちの長さの記録。テスト用）。nil なら time.Now。
	Now func() time.Time
	// Sleep は ErrBackgroundBlocked の待ち（ctx が終われば早く戻りエラーを返す）。nil なら time.After で待つ。テスト用。
	Sleep func(ctx context.Context, d time.Duration) error

	mu   sync.Mutex
	last map[string]time.Time
	// gaveUpAt は前面へ戻らないまま背面の待ちを諦めた時刻（待ちが通った・別の結果になったらゼロ値へ戻す）。mu で守る。
	gaveUpAt time.Time
}

// DialContext は Base で接続し、名前解決の失敗（*net.DNSError）だけを Fallback で救う。それ以外の失敗は Base のエラーのまま返す。
func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := d.Base(ctx, network, addr)
	if err == nil {
		return conn, nil
	}
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) {
		return nil, err
	}
	host, port, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		return nil, err
	}

	start := d.now()
	ips, lookupErr := d.lookupWaitingForForeground(ctx, host)
	ips = filterFamily(network, ips)
	if lookupErr != nil || len(ips) == 0 {
		d.logFailure(ctx, host, err, lookupErr, len(ips))
		return nil, err
	}
	var dialErr error
	for _, ip := range ips {
		if ctx.Err() != nil {
			break
		}
		c, e := d.dialOne(ctx, network, net.JoinHostPort(ip, port))
		if e == nil {
			d.logRecovered(host, err, d.now().Sub(start))
			return c, nil
		}
		dialErr = e
	}
	d.logFailure(ctx, host, err, dialErr, len(ips))
	return nil, err
}

// lookupWaitingForForeground は Fallback.LookupHost を呼ぶ。ErrBackgroundBlocked のあいだは、アプリが前面へ戻って通信が許されるのを
// backgroundWaitInterval ごとに最大 backgroundWaitAttempts 回まで待つ（ctx が終われば止める）。
// 待ちを諦めたあと backgroundWaitCooldown のあいだは待たず、1 回だけ引き直す（背面で圏外のときの、同期ごとの待ちを避ける）。
func (d *Dialer) lookupWaitingForForeground(ctx context.Context, host string) ([]string, error) {
	maxAttempts := backgroundWaitAttempts
	if d.inBackgroundWaitCooldown() {
		maxAttempts = 1
	}
	for attempt := 1; ; attempt++ {
		ips, err := d.Fallback.LookupHost(ctx, host)
		if !errors.Is(err, ErrBackgroundBlocked) {
			d.setGaveUpAt(time.Time{}) // 通った・別の結果になった（前面へ戻った）ので、次は待ってよい
			return ips, err
		}
		if attempt >= maxAttempts {
			if maxAttempts == backgroundWaitAttempts {
				d.setGaveUpAt(d.now())
			}
			return ips, err
		}
		if err := d.sleep(ctx, backgroundWaitInterval); err != nil {
			return nil, err
		}
	}
}

func (d *Dialer) inBackgroundWaitCooldown() bool {
	t := d.now()
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.gaveUpAt.IsZero() && t.Sub(d.gaveUpAt) < backgroundWaitCooldown
}

func (d *Dialer) setGaveUpAt(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.gaveUpAt = t
}

func (d *Dialer) sleep(ctx context.Context, wait time.Duration) error {
	if d.Sleep != nil {
		return d.Sleep(ctx, wait)
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Dialer) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Dialer) dialOne(ctx context.Context, network, addr string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	return d.Base(ctx, network, addr)
}

// filterFamily は network（tcp4・tcp6）が指す IP のファミリーだけを残す。tcp などはそのまま。IP として読めないものは捨てる。
func filterFamily(network string, ips []string) []string {
	out := make([]string, 0, len(ips))
	for _, s := range ips {
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		is4 := ip.To4() != nil
		if (network == "tcp4" && !is4) || (network == "tcp6" && is4) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (d *Dialer) logger() *slog.Logger {
	if d.Logger != nil {
		if l := d.Logger(); l != nil {
			return l
		}
	}
	return slog.Default()
}

// shouldLog は key のログを今出してよいか（直近 logInterval 以内に出していなければ真。出したことを記録する）。
func (d *Dialer) shouldLog(key string) bool {
	t := d.now()
	d.mu.Lock()
	defer d.mu.Unlock()
	if prev, ok := d.last[key]; ok && t.Sub(prev) < logInterval {
		return false
	}
	if d.last == nil {
		d.last = map[string]time.Time{}
	}
	d.last[key] = t
	return true
}

// logFailure は標準の解決も Android の API による引き直しも通らなかったことを、手がかりつきで Warn に残す。
// host はエラー文（`lookup <host>: …`）にも出る値。DNS サーバーのアドレスは端末の設定値（秘密ではない）。
// tried（`fallback_addrs`）は、引き直しで得たアドレスのうち network のファミリーで絞ったあとの件数。
// 呼び出し元の ctx の取り消し・期限だけが理由の失敗は、記録しない。
func (d *Dialer) logFailure(ctx context.Context, host string, sysErr, fallbackErr error, tried int) {
	if ctx.Err() != nil && (fallbackErr == nil || errors.Is(fallbackErr, context.Canceled) || errors.Is(fallbackErr, context.DeadlineExceeded)) {
		return // 呼び出し元の取り消し・期限で止まっただけ（引き直しの結果が出ていない）。失敗の記録にも 1 分の枠にも数えない
	}
	if !d.shouldLog("fail:" + host) {
		return
	}
	attrs := []any{"host", host, "resolver", d.Resolver, "err", sysErr}
	if fallbackErr != nil {
		attrs = append(attrs, "fallback_err", fallbackErr)
	}
	attrs = append(attrs, "fallback_addrs", tried)
	// 呼び出し元の ctx が取り消されていても、手がかりは取る（上限は diagnoseTimeout）。
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), diagnoseTimeout)
	defer cancel()
	diag, derr := d.Fallback.Diagnose(dctx)
	if derr != nil {
		attrs = append(attrs, "diagnose_err", derr)
	} else {
		attrs = append(attrs, diag.Attrs()...)
	}
	d.logger().Warn("net: name resolution failed", attrs...)
}

// logRecovered は標準の解決が失敗し、Android の API で引き直して接続できたことを Warn に残す（標準の解決器が壊れている印）。
// waited は ErrBackgroundBlocked の待ちを含む、救済にかかった時間。
func (d *Dialer) logRecovered(host string, sysErr error, waited time.Duration) {
	if !d.shouldLog("recovered:" + host) {
		return
	}
	d.logger().Warn("net: name resolution recovered via Android Network API", "host", host, "resolver", d.Resolver, "err", sysErr, "waited_ms", waited.Milliseconds())
}

// Install は t の DialContext を d の接続に差し替える。t が nil なら何もしない。
func Install(t *http.Transport, d *Dialer) {
	if t == nil {
		return
	}
	t.DialContext = d.DialContext
}
