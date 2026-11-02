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
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Path は MCP のエンドポイントのパス。
const Path = "/mcp"

// 接続まわりの上限。
const (
	readHeaderTimeout = 10 * time.Second
	shutdownGrace     = 2 * time.Second
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
	Addr      string // 実際に待ち受けているアドレス（Port 0 のときに決まったポートが入る）
	Err       string // 直近の待ち受け失敗（ポート使用中など）
}

// Manager は MCP サーバーの起動・停止を担う。
type Manager struct {
	opts   Options
	server *mcp.Server

	mu    sync.Mutex
	http  *http.Server
	done  chan struct{}
	state State
}

// New は Manager を作る。待ち受けは始めない（Apply が有効な設定を受けたときだけ始める）。
func New(opts Options) *Manager {
	return &Manager{opts: opts, server: newServer(opts.Backend, opts.LogDir, opts.Logger, opts.Version)}
}

// State は現在の状態を返す。
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Apply は cfg に合わせて待ち受けを作り直す。無効なら止めるだけ。待ち受けに失敗したらエラーを返し、停止状態にする。
// cfg の検証は呼び出し側が済ませる（Port 0 はテスト用に OS 任せで待ち受ける）。
func (m *Manager) Apply(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	if !cfg.Enabled {
		return nil
	}
	ln, err := net.Listen("tcp", cfg.Addr())
	if err != nil {
		m.state = State{Err: "待ち受けを開始できません。ポートが使われていないか確認してください。"}
		m.opts.Logger.Error("mcp listen failed", "err", err)
		return fmt.Errorf("mcp listen: %w", err)
	}
	srv := &http.Server{Handler: m.handler(), ReadHeaderTimeout: readHeaderTimeout}
	done := make(chan struct{})
	m.http, m.done = srv, done
	m.state = State{Listening: true, Addr: ln.Addr().String()}
	m.opts.Logger.Info("mcp listening", "addr", m.state.Addr)
	go func() {
		defer close(done)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			m.opts.Logger.Error("mcp serve failed", "err", err)
		}
	}()
	return nil
}

// Close は待ち受けを止める（アプリの終了時）。
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *Manager) stopLocked() {
	if m.http == nil {
		m.state = State{}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := m.http.Shutdown(ctx); err != nil {
		_ = m.http.Close() // 開きっぱなしのストリームは待たずに切る
	}
	<-m.done
	m.http, m.done, m.state = nil, nil, State{}
	m.opts.Logger.Info("mcp stopped")
}

// handler は Bearer トークン必須の MCP ハンドラ。トークンが未発行でも、合わなくても、ヘッダが無くても 401 を返す。
func (m *Manager) handler() http.Handler {
	verify := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if !m.opts.Tokens.Verify(token) {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{Expiration: time.Now().Add(tokenLifetime)}, nil
	}
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return m.server }, nil)
	guarded := auth.RequireBearerToken(verify, nil)(mcpHandler)
	mux := http.NewServeMux()
	mux.Handle(Path, m.logRejected(guarded))
	return mux
}

// logRejected は認証で拒否された接続を（接続元だけ）記録する。
func (m *Manager) logRejected(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.status == http.StatusUnauthorized || rec.status == http.StatusForbidden {
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
