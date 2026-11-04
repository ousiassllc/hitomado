package appcore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/store"
)

func TestParseInboxTab(t *testing.T) {
	for in, want := range map[string]store.InboxTab{
		"": store.InboxTabAll, "all": store.InboxTabAll, "primary": store.InboxTabPrimary, "social": store.InboxTabSocial,
		"promotions": store.InboxTabPromotions, "new": store.InboxTabNew,
	} {
		got, err := ParseInboxTab(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"updates", "forums", "ALL", "primary "} {
		_, err := ParseInboxTab(in)
		var ae *apperr.Error
		require.ErrorAs(t, err, &ae, in)
		assert.Equal(t, apperr.Validation, ae.Code)
	}
}
