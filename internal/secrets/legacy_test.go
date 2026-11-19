package secrets_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/secrets"
)

const (
	newKey = "nefos-inbox/refresh_token/acc1"
	oldKey = "hitomado/refresh_token/acc1"
)

func TestWithLegacyMovesValueOnFirstRead(t *testing.T) {
	cur, old := secrets.NewMemory(), secrets.NewMemory()
	require.NoError(t, old.Set(oldKey, "tok"))
	s := secrets.WithLegacy(cur, old)

	got, err := s.Get(newKey)
	require.NoError(t, err)
	assert.Equal(t, "tok", got)
	v, err := cur.Get(newKey)
	require.NoError(t, err)
	assert.Equal(t, "tok", v)
	_, err = old.Get(oldKey)
	require.ErrorIs(t, err, secrets.ErrNotFound)
}

func TestWithLegacyPrefersCurrentValue(t *testing.T) {
	cur, old := secrets.NewMemory(), secrets.NewMemory()
	require.NoError(t, cur.Set(newKey, "new"))
	require.NoError(t, old.Set(oldKey, "old"))

	got, err := secrets.WithLegacy(cur, old).Get(newKey)
	require.NoError(t, err)
	assert.Equal(t, "new", got)
	v, err := old.Get(oldKey)
	require.NoError(t, err)
	assert.Equal(t, "old", v, "新しい値があるときは旧値に触らない")
}

func TestWithLegacyMissingEverywhere(t *testing.T) {
	_, err := secrets.WithLegacy(secrets.NewMemory(), secrets.NewMemory()).Get(newKey)
	require.ErrorIs(t, err, secrets.ErrNotFound)
	// 接頭辞が違うキーは旧キー名を探さない。
	_, err = secrets.WithLegacy(secrets.NewMemory(), secrets.NewMemory()).Get("other/key")
	require.ErrorIs(t, err, secrets.ErrNotFound)
}

func TestWithLegacySameStoreForAndroid(t *testing.T) {
	m := secrets.NewMemory()
	require.NoError(t, m.Set(oldKey, "tok"))
	s := secrets.WithLegacy(m, m)

	got, err := s.Get(newKey)
	require.NoError(t, err)
	assert.Equal(t, "tok", got)
	_, err = m.Get(oldKey)
	require.ErrorIs(t, err, secrets.ErrNotFound)
	v, err := m.Get(newKey)
	require.NoError(t, err)
	assert.Equal(t, "tok", v)
}

// 書き写しに失敗したら、旧値は消さず、読めた値をそのまま返す（次の読み出しでもう一度試す）。
func TestWithLegacyKeepsOldValueWhenCopyFails(t *testing.T) {
	cur, old := &failSet{Store: secrets.NewMemory(), fail: true}, secrets.NewMemory()
	require.NoError(t, old.Set(oldKey, "tok"))
	s := secrets.WithLegacy(cur, old)

	got, err := s.Get(newKey)
	require.NoError(t, err)
	assert.Equal(t, "tok", got)
	v, err := old.Get(oldKey)
	require.NoError(t, err)
	assert.Equal(t, "tok", v)

	cur.fail = false
	got, err = s.Get(newKey)
	require.NoError(t, err)
	assert.Equal(t, "tok", got)
	_, err = old.Get(oldKey)
	require.ErrorIs(t, err, secrets.ErrNotFound, "移せたら旧値を消す")
}

func TestWithLegacyDeleteRemovesBoth(t *testing.T) {
	cur, old := secrets.NewMemory(), secrets.NewMemory()
	require.NoError(t, cur.Set(newKey, "new"))
	require.NoError(t, old.Set(oldKey, "old"))
	s := secrets.WithLegacy(cur, old)

	require.NoError(t, s.Delete(newKey))
	_, err := s.Get(newKey)
	require.ErrorIs(t, err, secrets.ErrNotFound, "消した値が旧キーから戻らない")
}

func TestWithLegacyReadErrorOfLegacyIsReported(t *testing.T) {
	boom := errors.New("backend down")
	s := secrets.WithLegacy(secrets.NewMemory(), errStore{err: boom})
	_, err := s.Get(newKey)
	require.ErrorIs(t, err, boom)
}

type failSet struct {
	secrets.Store
	fail bool
}

func (f *failSet) Set(key, value string) error {
	if !f.fail {
		return f.Store.Set(key, value)
	}
	return errors.New("set failed")
}

type errStore struct{ err error }

func (e errStore) Get(string) (string, error) { return "", e.err }
func (e errStore) Set(string, string) error   { return e.err }
func (e errStore) Delete(string) error        { return e.err }
