package siwctest

import (
	"context"
	"fmt"
	"sync"

	"github.com/ousiassllc/hitomado/internal/secrets"
)

// テスト用の秘密らしい値。ログ・エラー・戻り値に出てはならない。
const (
	SecretAccess  = "ACCESS-SECRET-VALUE-1"
	SecretRefresh = "REFRESH-SECRET-VALUE-1"
	IssuedClient  = "oaiapp_TESTCLIENT"
	SubjectA      = "sub-account-a"
	ScopeAll      = "chatgpt.tokens.use.direct email offline_access openid profile resource.invoke"
)

// MemSettings は siwc.Settings のメモリ実装。
type MemSettings struct {
	mu sync.Mutex
	m  map[string]string
}

func (s *MemSettings) GetSetting(_ context.Context, key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	return v, ok, nil
}

func (s *MemSettings) SetSetting(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[key] = value
	return nil
}

// LimitedSecrets は Windows 資格情報マネージャーと同じ 1 件 2560 バイトの上限を持つ資格情報ストア。
type LimitedSecrets struct {
	*secrets.Memory
	Max int
}

func (l LimitedSecrets) Set(key, value string) error {
	if len(value) > l.Max {
		return fmt.Errorf("too big: %d", len(value))
	}
	return l.Memory.Set(key, value)
}
