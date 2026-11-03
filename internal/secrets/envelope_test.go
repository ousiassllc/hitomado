package secrets

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEnvelopeJava は Java の SecretBridge（secureGet／secureSet／secureDelete）を模した呼び出し口。
type fakeEnvelopeJava struct {
	mu      sync.Mutex
	m       map[string]string
	calls   []string
	fail    string // 空でなければ全操作が {"ok":false,"error":fail}
	dropped bool   // true なら呼び出し自体が失敗（ok=false）
	args    []string
}

func (f *fakeEnvelopeJava) call(method, arg string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, method)
	f.args = append(f.args, arg)
	if f.dropped {
		return "", false
	}
	if f.fail != "" {
		return `{"ok":false,"error":"` + f.fail + `"}`, true
	}
	switch method {
	case "secureGet":
		v, ok := f.m[arg]
		if !ok {
			return `{"ok":true,"found":false}`, true
		}
		b, _ := json.Marshal(map[string]any{"ok": true, "found": true, "value": v})
		return string(b), true
	case "secureSet":
		var in struct{ Key, Value string }
		if err := json.Unmarshal([]byte(arg), &in); err != nil {
			return `{"ok":false,"error":"failed"}`, true
		}
		f.m[in.Key] = in.Value
		return `{"ok":true}`, true
	case "secureDelete":
		delete(f.m, arg)
		return `{"ok":true}`, true
	}
	return "", false
}

func newEnvelopeStore(f *fakeEnvelopeJava) Store {
	f.m = map[string]string{}
	return NewBridgeStore(NewEnvelopeBridge(f.call))
}

func TestEnvelopeStoreRoundTrip(t *testing.T) {
	f := &fakeEnvelopeJava{}
	s := newEnvelopeStore(f)
	key := RefreshTokenKey("acct1")

	_, err := s.Get(key)
	assert.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, s.Set(key, "1//token-値"))
	got, err := s.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "1//token-値", got)

	require.NoError(t, s.Delete(key))
	_, err = s.Get(key)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestEnvelopeStoreSendsASCIIOnly(t *testing.T) {
	f := &fakeEnvelopeJava{}
	s := newEnvelopeStore(f)
	require.NoError(t, s.Set("k", "値😀"))
	for _, r := range f.args[0] {
		require.Less(t, r, rune(0x80), "JNI の NewStringUTF のため ASCII だけ: %q", f.args[0])
	}
}

func TestEnvelopeStoreMapsKeyLost(t *testing.T) {
	f := &fakeEnvelopeJava{fail: "key_lost"}
	s := newEnvelopeStore(f)
	_, err := s.Get("k")
	assert.ErrorIs(t, err, ErrKeyLost)
	assert.ErrorIs(t, err, ErrNotFound, "鍵の喪失は「値が無い」と同じに扱える")
}

func TestEnvelopeStoreOtherFailuresDoNotLeakValues(t *testing.T) {
	f := &fakeEnvelopeJava{fail: "failed"}
	s := newEnvelopeStore(f)
	err := s.Set("k", "secret-value")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-value")
	assert.False(t, errors.Is(err, ErrKeyLost))

	f.dropped = true
	_, err = s.Get("k")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotFound), "呼び出しの失敗を「無い」と取り違えない")
}

func TestEnvelopeStoreRejectsEmptyKeyWithoutCalling(t *testing.T) {
	f := &fakeEnvelopeJava{}
	s := newEnvelopeStore(f)
	_, err := s.Get("")
	assert.Error(t, err)
	assert.Error(t, s.Set("", "v"))
	assert.Error(t, s.Delete(""))
	assert.Empty(t, f.calls)
}

func TestEnvelopeStoreMalformedResponse(t *testing.T) {
	s := NewBridgeStore(NewEnvelopeBridge(func(string, string) (string, bool) { return "not json", true }))
	_, err := s.Get("k")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotFound))
}
