package secrets

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeJava は Java の SecureStore（Wails の application.Android 経由）を模した bridge。
// Wails は Java の失敗コードを errors.New("secure storage: " + code) にして返す。
type fakeJava struct {
	mu      sync.Mutex
	m       map[string]string
	keyLost bool  // true の間、Get は key_lost を返し、返した時点で内容を空にして false に戻る（Java のストア作り直し）
	failErr error // 非 nil なら全操作がこれを返す
}

func newFakeJava() *fakeJava { return &fakeJava{m: map[string]string{}} }

func (f *fakeJava) SecureGet(key string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failErr != nil {
		return "", false, f.failErr
	}
	if f.keyLost {
		f.keyLost, f.m = false, map[string]string{}
		return "", false, errors.New(bridgeErrPrefix + bridgeCodeKeyLost)
	}
	v, ok := f.m[key]
	return v, ok, nil
}

func (f *fakeJava) SecureSet(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failErr != nil {
		return f.failErr
	}
	f.m[key] = value
	return nil
}

func (f *fakeJava) SecureDelete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failErr != nil {
		return f.failErr
	}
	delete(f.m, key)
	return nil
}

func TestBridgeStoreRoundTripAndDelete(t *testing.T) {
	s := newBridgeStore(newFakeJava())
	key := RefreshTokenKey("acc1")

	_, err := s.Get(key)
	require.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, s.Set(key, "v1"))
	got, err := s.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "v1", got)

	require.NoError(t, s.Set(key, "v2")) // 上書き
	got, err = s.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "v2", got)

	require.NoError(t, s.Delete(key))
	_, err = s.Get(key)
	require.ErrorIs(t, err, ErrNotFound)
	assert.NotErrorIs(t, err, ErrKeyLost)
}

func TestBridgeStoreDeleteMissingIsNil(t *testing.T) {
	require.NoError(t, newBridgeStore(newFakeJava()).Delete(AIAPIKeyKey("openai")))
}

func TestBridgeStoreLargeValue(t *testing.T) {
	// ChatGPT のトークン（JWT）は 4096 バイトを超えうる。分割せずそのまま往復できる。
	s := newBridgeStore(newFakeJava())
	big := strings.Repeat("a1b2", 2048) + "終"
	require.NoError(t, s.Set(ChatGPTTokenKey("oaiapp_test", "access"), big))
	got, err := s.Get(ChatGPTTokenKey("oaiapp_test", "access"))
	require.NoError(t, err)
	assert.Equal(t, big, got)
}

func TestBridgeStoreKeysAreIndependent(t *testing.T) {
	s := newBridgeStore(newFakeJava())
	require.NoError(t, s.Set(RefreshTokenKey("a"), "rt-a"))
	require.NoError(t, s.Set(RefreshTokenKey("b"), "rt-b"))
	require.NoError(t, s.Set(AIAPIKeyKey("a"), "ai-a"))
	require.NoError(t, s.Delete(RefreshTokenKey("a")))

	_, err := s.Get(RefreshTokenKey("a"))
	require.ErrorIs(t, err, ErrNotFound)
	for key, want := range map[string]string{RefreshTokenKey("b"): "rt-b", AIAPIKeyKey("a"): "ai-a"} {
		got, err := s.Get(key)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestBridgeStoreKeyLostIsNotFoundAndKeyLost(t *testing.T) {
	f := newFakeJava()
	s := newBridgeStore(f)
	require.NoError(t, s.Set(RefreshTokenKey("a"), "rt-a"))
	f.keyLost = true

	_, err := s.Get(RefreshTokenKey("a"))
	require.ErrorIs(t, err, ErrKeyLost)
	// 既存の呼び出し側（gmail・aicore・mcpserver・siwc・app）は errors.Is(err, ErrNotFound) で「無い」を扱う。
	require.ErrorIs(t, err, ErrNotFound)

	// ストアは作り直し済み。以後は普通の「無い」で、新しい値を保存して読める（再接続で復旧する）。
	_, err = s.Get(RefreshTokenKey("a"))
	require.ErrorIs(t, err, ErrNotFound)
	assert.NotErrorIs(t, err, ErrKeyLost)
	require.NoError(t, s.Set(RefreshTokenKey("a"), "rt-new"))
	got, err := s.Get(RefreshTokenKey("a"))
	require.NoError(t, err)
	assert.Equal(t, "rt-new", got)
}

func TestBridgeStoreOtherFailuresAreNotNotFound(t *testing.T) {
	boom := errors.New(bridgeErrPrefix + "failed")
	f := newFakeJava()
	f.failErr = boom
	s := newBridgeStore(f)

	_, err := s.Get("k")
	require.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, ErrNotFound) // 読めない原因が鍵の喪失でないなら、再ログインへ案内しない
	assert.NotErrorIs(t, err, ErrKeyLost)

	err = s.Set("k", "very-secret-value")
	require.ErrorIs(t, err, boom)
	assert.NotContains(t, err.Error(), "very-secret-value")

	require.ErrorIs(t, s.Delete("k"), boom)
}
