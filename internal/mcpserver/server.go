// Package mcpserver は Hitomado に内蔵する MCP サーバー（Streamable HTTP）を提供する。
//
// 既定はオフで、Manager.Apply で有効にしたときだけ待ち受ける。待ち受けても Bearer トークンが無い・合わない接続は
// 401 で拒否する。公開するツールは読み取りと返信下書きの作成だけで、送信するツールは無い（ToolNames）。
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"

	"github.com/ousiassllc/hitomado/internal/logging"
)

// Path は MCP のエンドポイントのパス。
const Path = "/mcp"

// 接続まわりの上限。
const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 2 * time.Minute
	sessionTimeout    = 30 * time.Minute // 使われなくなった MCP セッションを閉じる（溜まり続けない）
	shutdownGrace     = 2 * time.Second
	// 拒否の記録は 1 秒に 1 件（瞬間 5 件）まで。連打でログの履歴が押し出されないようにする。
	rejectLogBurst = 5
	// tokenLifetime は verifier が返す有効期限。トークン自体に期限は無い（再発行・無効化で切る）ので、SDK が要求する値を遠い先にする。
	tokenLifetime = 24 * time.Hour
)

// Options は Manager の依存。
type Options struct {
	Backend Backend
	Tokens  *Tokens
	Logger  *slog.Logger
	LogDir  string // get_logs / get_status が読むログフォルダ
	Version string
}

// State は待ち受けの状態。
type State struct {
	Listening bool
	Addr      string // 実際に待ち受けているアドレス
	Err       string // 直近の待ち受け失敗（ポート使用中など）
}

// Manager は MCP サーバーの起動・停止を担う。
type Manager struct {
	opts      Options
	server    *mcp.Server
	rejectLog *rate.Limiter

	mu     sync.Mutex // Apply／Close を直列にする（停止の待ちの間も持つ）
	http   *http.Server
	done   chan struct{}
	closed bool

	stateMu sync.Mutex // state だけを守る（停止の待ちの間でも State が止まらない）
	state   State
}

// New は Manager を作る。待ち受けは始めない（Apply が有効な設定を受けたときだけ始める）。
func New(opts Options) *Manager {
	return &Manager{
		opts: opts, server: newServer(opts.Backend, opts.LogDir, opts.Logger, opts.Version),
		rejectLog: rate.NewLimiter(rate.Every(time.Second), rejectLogBurst),
	}
}

// State は現在の状態を返す。
func (m *Manager) State() State {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.state
}

func (m *Manager) setState(st State) {
	m.stateMu.Lock()
	m.state = st
	m.stateMu.Unlock()
}

// Apply は cfg に合わせて待ち受けを作り直す（開いている接続は切れる）。無効なら止めるだけ。
// 有効にするときは cfg を検証し、満たさない設定（loopback 以外で許可なし・範囲外のポートなど）では待ち受けない。
// 待ち受けに失敗したらエラーを返し、停止状態にする。Close のあとは何もしない（待ち受けない）。
func (m *Manager) Apply(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	if !cfg.Enabled || m.closed {
		return nil
	}
	if err := cfg.Validate(); err != nil {
		m.setState(State{Err: err.Error()})
		m.opts.Logger.Warn("mcp config rejected", "err", err)
		return fmt.Errorf("mcp config: %w", err)
	}
	ln, err := net.Listen("tcp", cfg.Addr())
	if err != nil {
		m.setState(State{Err: "待ち受けを開始できません。ポートが使われていないか確認してください。"})
		m.opts.Logger.Error("mcp listen failed", "err", err)
		return fmt.Errorf("mcp listen: %w", err)
	}
	srv := &http.Server{
		Handler: m.handler(cfg), ReadHeaderTimeout: readHeaderTimeout, IdleTimeout: idleTimeout,
		ErrorLog: slog.NewLogLogger(m.opts.Logger.Handler(), slog.LevelWarn),
	}
	done := make(chan struct{})
	m.http, m.done = srv, done
	m.setState(State{Listening: true, Addr: ln.Addr().String()})
	m.opts.Logger.Info("mcp listening", "addr", ln.Addr().String())
	go func() {
		defer close(done)
		defer logging.Recover(m.opts.Logger, "mcp serve")
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			m.opts.Logger.Error("mcp serve failed", "err", err)
		}
	}()
	return nil
}

// Close は待ち受けを止める（アプリの終了時）。以後 Apply は待ち受けない。
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.stopLocked()
}

func (m *Manager) stopLocked() {
	if m.http == nil {
		m.setState(State{})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := m.http.Shutdown(ctx); err != nil {
		_ = m.http.Close() // 開きっぱなしのストリームは待たずに切る
	}
	<-m.done
	m.http, m.done = nil, nil
	m.setState(State{})
	m.opts.Logger.Info("mcp stopped")
}

// handler は Bearer トークン必須の MCP ハンドラ。トークンが未発行でも、合わなくても、ヘッダが無くても 401 を返す。
// その前段で、DNS rebinding・ブラウザからの横断リクエストを Host／Origin の検証で断つ（hostGuard）。
func (m *Manager) handler(cfg Config) http.Handler {
	verify := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if !m.opts.Tokens.Verify(token) {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{Expiration: time.Now().Add(tokenLifetime)}, nil
	}
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return m.server },
		&mcp.StreamableHTTPOptions{SessionTimeout: sessionTimeout})
	guarded := auth.RequireBearerToken(verify, nil)(mcpHandler)
	mux := http.NewServeMux()
	mux.Handle(Path, m.logRejected(hostGuard(cfg.Host, guarded)))
	return mux
}

// hostGuard は Host が「IP アドレスそのもの・localhost・設定のホスト名」でない要求と、Origin が Host と同じでない
// 要求を 403 にする。MCP クライアントは Origin を付けないので影響しない。DNS rebinding（攻撃者のドメインが
// 一時的に待ち受けアドレスを指す）はドメイン名の Host になるため、loopback 以外で待ち受けるときもここで断てる。
func hostGuard(cfgHost string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, cfgHost) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !sameOrigin(o, r.Host) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hostAllowed(hostHeader, cfgHost string) bool {
	h := hostHeader
	if host, _, err := net.SplitHostPort(hostHeader); err == nil {
		h = host
	}
	h = strings.Trim(h, "[]")
	return net.ParseIP(h) != nil || strings.EqualFold(h, "localhost") || strings.EqualFold(h, strings.Trim(cfgHost, "[]"))
}

func sameOrigin(origin, hostHeader string) bool {
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && strings.EqualFold(u.Host, hostHeader)
}

// logRejected は認証で拒否された接続を（接続元だけ。1 秒に 1 件まで）記録する。
func (m *Manager) logRejected(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if (rec.status == http.StatusUnauthorized || rec.status == http.StatusForbidden) && m.rejectLog.Allow() {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			m.opts.Logger.Warn("mcp connection rejected", "remote", host, "status", rec.status)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush はストリーミング応答（SSE）のために元の Flusher へ渡す。
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap は http.ResponseController 用。
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
