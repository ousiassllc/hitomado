// Package siwctest は siwc のテスト用の偽 OpenAI（認可サーバー）と、それに向けた Manager の環境を提供する（siwctests が使う）。
package siwctest

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
	"github.com/ousiassllc/hitomado/internal/store"
)

// Env は Manager とその周辺。
type Env struct {
	F        *FakeOpenAI
	Sec      secrets.Store
	Settings *MemSettings
	Clk      *clock.Fake
	M        *siwc.Manager
	Logs     *SyncBuilder
}

type SyncBuilder struct {
	Mu sync.Mutex
	sb strings.Builder
}

func (s *SyncBuilder) Write(p []byte) (int, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.sb.Write(p)
}

func (s *SyncBuilder) String() string {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.sb.String()
}

// NewEnv は偽 OpenAI に向けた Manager を作る。sec が nil ならメモリの資格情報ストア。
func NewEnv(t *testing.T, sec secrets.Store, mutate func(*siwc.Options)) *Env {
	t.Helper()
	f := NewFakeOpenAI(t)
	if sec == nil {
		sec = secrets.NewMemory()
	}
	clk := clock.NewFake(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC))
	f.Clk = clk
	logs := &SyncBuilder{}
	e := &Env{F: f, Sec: sec, Settings: &MemSettings{}, Clk: clk, Logs: logs}
	opts := siwc.Options{
		OpenBrowser: f.Browser(t), Secrets: sec, Settings: e.Settings, Clock: clk,
		Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Issuer: f.Srv.URL, HTTPClient: f.Srv.Client(), Timeout: 5 * time.Second, RevokeBackoff: time.Millisecond,
	}
	if mutate != nil {
		mutate(&opts)
	}
	e.M = siwc.New(opts)
	return e
}

// SignIn は新規登録でログインする。
func (e *Env) SignIn(t *testing.T) siwc.Status {
	t.Helper()
	st, err := e.M.SignIn(t.Context(), siwc.SignInOptions{})
	require.NoError(t, err)
	return st
}

// TokenKey は IssuedClient（既定で発行される client_id）のトークンのキー。
func TokenKey(kind string) string { return secrets.ChatGPTTokenKey(IssuedClient, kind) }

// Relogin は IssuedClient の登録で再ログインする。
func (e *Env) Relogin(t *testing.T) siwc.Status {
	t.Helper()
	st, err := e.M.SignIn(t.Context(), siwc.SignInOptions{AccountID: IssuedClient})
	require.NoError(t, err)
	return st
}

// SavedAcct・SavedReg は app_settings の ai.chatgpt（秘密を含まない登録）。
type SavedAcct struct {
	ClientID      string `json:"clientId"`
	Subject       string `json:"subject"`
	Email         string `json:"email"`
	ConsentNeeded bool   `json:"consentNeeded"`
	ExpiresAt     int64  `json:"expiresAt"`
	PlanNotice    string `json:"planNotice"`
}

type SavedReg struct {
	HostID   string      `json:"hostId"`
	Active   string      `json:"active"`
	Accounts []SavedAcct `json:"accounts"`
}

// First は最初のアカウント（無ければ零値）。
func (r SavedReg) First() SavedAcct {
	if len(r.Accounts) == 0 {
		return SavedAcct{}
	}
	return r.Accounts[0]
}

func (e *Env) Reg(t *testing.T) SavedReg {
	t.Helper()
	v, ok, _ := e.Settings.GetSetting(t.Context(), store.SettingAIChatGPT)
	require.True(t, ok)
	var r SavedReg
	require.NoError(t, json.Unmarshal([]byte(v), &r))
	return r
}
