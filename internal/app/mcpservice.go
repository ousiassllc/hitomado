package app

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/mcpserver"
	"github.com/ousiassllc/hitomado/internal/store"
)

// MCPService は内蔵 MCP サーバーの設定・トークン・起動停止（バインディング仕様 §9）。
// 既定はオフ。オンにしても Bearer トークンが無い接続は拒否する。公開するツールは mcpbackend.go の読み取りと返信下書きの作成だけ。
type MCPService struct {
	d      *Deps
	mgr    *mcpserver.Manager
	tokens *mcpserver.Tokens

	mu sync.Mutex // 設定の保存と Apply を直列にする
}

// NewMCPService は MCPService を作る（待ち受けは Start まで始めない）。
func NewMCPService(d *Deps, backend mcpserver.Backend) *MCPService {
	tokens := mcpserver.NewTokens(d.Secrets)
	return &MCPService{
		d: d, tokens: tokens,
		mgr: mcpserver.New(mcpserver.Options{
			Backend: backend, Tokens: tokens, Logger: logger(d), LogDir: logDir(d), Version: d.Version,
		}),
	}
}

// Start は保存済みの設定が有効なときだけ待ち受けを始める。失敗してもアプリは続ける（状態に理由が出る）。
// 保存された値を信用せず、Manager.Apply が検証する（DB が書き換えられていても、許可の無い設定では待ち受けない）。
// Start と Shutdown はアプリの起動・終了処理（run.go）用で、フロントには公開しない。
//
//wails:ignore
func (s *MCPService) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.load(ctx)
	if err != nil {
		logger(s.d).Warn("MCP の設定を読めません。オフのままにします", "err", err)
		return
	}
	if cfg.Enabled {
		_ = s.mgr.Apply(cfg) // 失敗は Manager が記録し、GetMCPStatus が理由を返す
	}
}

// Shutdown は待ち受けを止める。
//
//wails:ignore
func (s *MCPService) Shutdown() { s.mgr.Close() }

// GetMCPStatus は設定と待ち受けの状態を返す。
func (s *MCPService) GetMCPStatus(ctx context.Context) (dto.MCPStatus, error) {
	cfg, err := s.load(ctx)
	if err != nil {
		return dto.MCPStatus{}, apperr.FromInfra(err, "")
	}
	return s.status(cfg), nil
}

// SetMCPConfig は設定を検証して保存し、待ち受けに反映する。待ち受けに失敗しても設定は保存し、状態の Error に理由を入れる。
func (s *MCPService) SetMCPConfig(ctx context.Context, in dto.MCPConfig) (dto.MCPStatus, error) {
	cfg := mcpserver.Config{Enabled: in.Enabled, Host: in.Host, Port: in.Port, AllowNonLoopback: in.AllowNonLoopback}.Normalize()
	// 止めるときは検証しない（不正な値が残っていてもオフにできる）。値は保存済みのものを維持する。
	if !cfg.Enabled {
		saved, err := s.load(ctx)
		if err != nil {
			return dto.MCPStatus{}, apperr.FromInfra(err, "")
		}
		saved.Enabled = false
		return s.save(ctx, saved)
	}
	if err := cfg.Validate(); err != nil {
		return dto.MCPStatus{}, validation(err.Error())
	}
	return s.save(ctx, cfg)
}

// save は設定を保存して待ち受けに反映する。待ち受けに失敗しても設定は保存し、状態の Error に理由を入れる。
func (s *MCPService) save(ctx context.Context, cfg mcpserver.Config) (dto.MCPStatus, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return dto.MCPStatus{}, apperr.FromInfra(err, "")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.d.Store.SetSetting(ctx, store.SettingMCPConfig, string(b)); err != nil {
		return dto.MCPStatus{}, apperr.FromInfra(err, "")
	}
	_ = s.mgr.Apply(cfg)
	return s.status(cfg), nil
}

// IssueMCPToken はトークンを（再）発行する。古いトークンは無効になる。値を返すのはこの 1 回だけ。
func (s *MCPService) IssueMCPToken(ctx context.Context) (dto.MCPTokenResult, error) {
	cfg, err := s.load(ctx)
	if err != nil {
		return dto.MCPTokenResult{}, apperr.FromInfra(err, "")
	}
	token, err := s.tokens.Issue()
	if err != nil {
		return dto.MCPTokenResult{}, apperr.Wrap(apperr.Internal, "トークンを発行できません。", err)
	}
	s.dropConnections(cfg)
	logger(s.d).Info("mcp token issued")
	return dto.MCPTokenResult{Token: token, Status: s.status(cfg)}, nil
}

// RevokeMCPToken はトークンを無効にする（以後すべての接続を拒否する）。
func (s *MCPService) RevokeMCPToken(ctx context.Context) (dto.MCPStatus, error) {
	cfg, err := s.load(ctx)
	if err != nil {
		return dto.MCPStatus{}, apperr.FromInfra(err, "")
	}
	if err := s.tokens.Revoke(); err != nil {
		return dto.MCPStatus{}, apperr.Wrap(apperr.Internal, "トークンを無効にできません。", err)
	}
	s.dropConnections(cfg)
	logger(s.d).Info("mcp token revoked")
	return s.status(cfg), nil
}

// dropConnections はトークンの再発行・無効化のあと、待ち受け中なら待ち受けを作り直して開いている接続・ストリームを切る
// （各リクエストはトークンを検証されるが、確立済みの SSE ストリームは検証されないため）。
func (s *MCPService) dropConnections(cfg mcpserver.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mgr.State().Listening {
		_ = s.mgr.Apply(cfg)
	}
}

// load は保存済みの設定を返す。未保存・壊れた値は既定（オフ）。
func (s *MCPService) load(ctx context.Context) (mcpserver.Config, error) {
	raw, ok, err := s.d.Store.GetSetting(ctx, store.SettingMCPConfig)
	if err != nil {
		return mcpserver.Config{}, err
	}
	var cfg mcpserver.Config
	if ok {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			logger(s.d).Warn("保存済みの MCP 設定を読めません。既定（オフ）を使います", "err", err)
			cfg = mcpserver.Config{}
		}
	}
	return cfg.Normalize(), nil
}

func (s *MCPService) status(cfg mcpserver.Config) dto.MCPStatus {
	st := s.mgr.State()
	return dto.MCPStatus{
		MCPConfig: dto.MCPConfig{Enabled: cfg.Enabled, Host: cfg.Host, Port: cfg.Port, AllowNonLoopback: cfg.AllowNonLoopback},
		HasToken:  s.tokens.Has(), Listening: st.Listening, Endpoint: cfg.EndpointURL(), Error: st.Err,
	}
}
