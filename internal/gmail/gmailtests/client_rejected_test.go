package gmailtests

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/gmail"
)

// OAuth クライアントの資格情報が拒否されたとき（invalid_client 等）は、再接続を促す ErrNeedsReauth とも
// 汎用の ErrGeneric とも区別し、再試行しない。応答本文は文言に含めない。
func TestRejectedOAuthClientIsDistinct(t *testing.T) {
	const secretBody = "client secret leaked in body"
	tests := []struct {
		name   string
		status int
		code   string
	}{
		{"invalid_client", 401, "invalid_client"},
		{"unauthorized_client", 400, "unauthorized_client"},
		{"401 without error code", 401, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEnv(t)
			var calls atomic.Int32
			e.f.Token = func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeJSON(w, tt.status, map[string]any{"error": tt.code, "error_description": secretBody})
			}
			_, err := e.c.Profile(context.Background(), testAccount)
			require.ErrorIs(t, err, gmail.ErrClientRejected)
			assert.NotErrorIs(t, err, gmail.ErrNeedsReauth)
			assert.NotErrorIs(t, err, gmail.ErrGeneric)
			assert.NotContains(t, err.Error(), secretBody)
			assert.Equal(t, int32(1), calls.Load(), "再試行しない")
		})
	}
}
