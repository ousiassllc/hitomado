package siwctests

import (
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/secrets"
	"github.com/ousiassllc/hitomado/internal/siwc"
)

// env は Manager とその周辺。
type env struct {
	f        *fakeOpenAI
	sec      secrets.Store
	settings *memSettings
	clk      *clock.Fake
	m        *siwc.Manager
	logs     *syncBuilder
}

type syncBuilder struct {
	mu sync.Mutex
	sb strings.Builder
}

func (s *syncBuilder) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sb.Write(p)
}

func (s *syncBuilder) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sb.String()
}

// newEnv は偽 OpenAI に向けた Manager を作る。sec が nil ならメモリの資格情報ストア。
func newEnv(t *testing.T, sec secrets.Store, mutate func(*siwc.Options)) *env {
	t.Helper()
	f := newFakeOpenAI(t)
	if sec == nil {
		sec = secrets.NewMemory()
	}
	clk := clock.NewFake(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC))
	f.clk = clk
	logs := &syncBuilder{}
	e := &env{f: f, sec: sec, settings: &memSettings{}, clk: clk, logs: logs}
	opts := siwc.Options{
		OpenBrowser: f.browser(t), Secrets: sec, Settings: e.settings, Clock: clk,
		Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Issuer: f.srv.URL, HTTPClient: f.srv.Client(), Timeout: 5 * time.Second, RevokeBackoff: time.Millisecond,
	}
	if mutate != nil {
		mutate(&opts)
	}
	e.m = siwc.New(opts)
	return e
}

// signIn は新規登録でログインする。
func (e *env) signIn(t *testing.T) siwc.Status {
	t.Helper()
	st, err := e.m.SignIn(t.Context(), siwc.SignInOptions{})
	require.NoError(t, err)
	return st
}
