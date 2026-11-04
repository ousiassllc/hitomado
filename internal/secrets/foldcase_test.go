package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFoldKeyCaseTreatsCaseVariantsAsTheSameKey(t *testing.T) {
	s := FoldKeyCase(NewMemory())
	require.NoError(t, s.Set(ChatGPTTokenKey("oaiapp_Abc", "access"), "v"))
	got, err := s.Get(ChatGPTTokenKey("oaiapp_abc", "access"))
	require.NoError(t, err)
	assert.Equal(t, "v", got)
	require.NoError(t, s.Delete(ChatGPTTokenKey("OAIAPP_ABC", "access")))
	_, err = s.Get(ChatGPTTokenKey("oaiapp_Abc", "access"))
	assert.ErrorIs(t, err, ErrNotFound)
}
