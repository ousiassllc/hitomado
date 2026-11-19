//go:build !android

package secrets_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/ousiassllc/nefos-inbox/internal/secrets"
)

func TestKeyringRoundTrip(t *testing.T) {
	keyring.MockInit()
	s := secrets.NewKeyring()
	key := secrets.RefreshTokenKey("acc1")

	_, err := s.Get(key)
	require.ErrorIs(t, err, secrets.ErrNotFound)

	require.NoError(t, s.Set(key, "v1"))
	got, err := s.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "v1", got)

	// service=Nefos Inbox, user=キー名で保存される。
	direct, err := keyring.Get("Nefos Inbox", key)
	require.NoError(t, err)
	assert.Equal(t, "v1", direct)

	require.NoError(t, s.Set(key, "v2"))
	got, err = s.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "v2", got)

	require.NoError(t, s.Delete(key))
	_, err = s.Get(key)
	require.ErrorIs(t, err, secrets.ErrNotFound)
}

func TestKeyringDeleteMissingIsNil(t *testing.T) {
	keyring.MockInit()
	require.NoError(t, secrets.NewKeyring().Delete(secrets.AIAPIKeyKey("openai")))
}

func TestKeyringErrorsAreWrappedWithoutValue(t *testing.T) {
	boom := errors.New("backend down")
	keyring.MockInitWithError(boom)
	t.Cleanup(keyring.MockInit)
	s := secrets.NewKeyring()

	_, err := s.Get("k")
	require.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, secrets.ErrNotFound)

	err = s.Set("k", "very-secret-value")
	require.ErrorIs(t, err, boom)
	assert.NotContains(t, err.Error(), "very-secret-value")

	require.ErrorIs(t, s.Delete("k"), boom)
}

func TestKeyringTooBigPassesThrough(t *testing.T) {
	keyring.MockInitWithError(keyring.ErrSetDataTooBig)
	t.Cleanup(keyring.MockInit)
	err := secrets.NewKeyring().Set("k", "v")
	require.ErrorIs(t, err, keyring.ErrSetDataTooBig)
}

// v0.1.46 までの保存先（service=Hitomado, user=hitomado/…）の値は、読んだときに新しい保存先へ移り、再ログインは要らない。
func TestKeyringMigratesLegacyCredentials(t *testing.T) {
	keyring.MockInit()
	require.NoError(t, keyring.Set("Hitomado", "hitomado/refresh_token/acc1", "tok"))
	require.NoError(t, keyring.Set("Hitomado", "hitomado/chatgpt_token/cid/access#0", "chunk"))
	s := secrets.NewKeyring()

	got, err := s.Get(secrets.RefreshTokenKey("acc1"))
	require.NoError(t, err)
	assert.Equal(t, "tok", got)
	moved, err := keyring.Get("Nefos Inbox", "nefos-inbox/refresh_token/acc1")
	require.NoError(t, err)
	assert.Equal(t, "tok", moved)
	_, err = keyring.Get("Hitomado", "hitomado/refresh_token/acc1")
	require.ErrorIs(t, err, keyring.ErrNotFound, "移したあとは旧名の項目を残さない")

	got, err = s.Get(secrets.ChatGPTTokenKey("cid", "access") + "#0")
	require.NoError(t, err)
	assert.Equal(t, "chunk", got, "分割保存の接尾辞も引き継ぐ")
}
