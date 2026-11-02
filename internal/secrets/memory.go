package secrets

import "sync"

// Memory はテスト・開発用のインメモリ実装（永続化しない）。並行利用できる。
type Memory struct {
	mu sync.RWMutex
	m  map[string]string
}

// NewMemory は空の Memory を返す。
func NewMemory() *Memory { return &Memory{m: map[string]string{}} }

// Get は値を返す。無ければ ErrNotFound。
func (s *Memory) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set は値を保存する。
func (s *Memory) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}

// Delete は値を削除する。無くてもエラーにしない。
func (s *Memory) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}
