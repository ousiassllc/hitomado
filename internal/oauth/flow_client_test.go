package oauth_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/oauth"
)

func TestAuthorizeTokenEndpointClientRejected(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"invalid_client":      {http.StatusUnauthorized, `{"error":"invalid_client","error_description":"` + testRefresh + `"}`},
		"unauthorized_client": {http.StatusBadRequest, `{"error":"unauthorized_client"}`},
		"401 本文なし":            {http.StatusUnauthorized, `not json ` + testRefresh},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			g := newFakeGoogle(t)
			g.tokenStatus = tc.status
			g.tokenBody = tc.body
			b := &browser{g: g, t: t, behave: okCallback}

			_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
			require.ErrorIs(t, err, oauth.ErrClientRejected)
			for _, secret := range []string{testRefresh, testCode, testClientSecret, b.params.State} {
				assert.NotContains(t, err.Error(), secret)
			}
		})
	}
}

func TestAuthorizeTokenEndpointOtherErrorIsNotClientRejected(t *testing.T) {
	g := newFakeGoogle(t)
	g.tokenStatus = http.StatusBadRequest // 既定本文は invalid_grant
	b := &browser{g: g, t: t, behave: okCallback}

	_, err := g.flow(b.open, time.Minute).Authorize(context.Background(), cfg)
	require.Error(t, err)
	assert.NotErrorIs(t, err, oauth.ErrClientRejected)
}
