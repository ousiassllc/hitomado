package secrets_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/ousiassllc/hitomado/internal/secrets"
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

	// service=Hitomado, user=キー名で保存される。
	direct, err := keyring.Get("Hitomado", key)
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
