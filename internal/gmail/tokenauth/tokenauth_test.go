package tokenauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/ousiassllc/hitomado/internal/clock"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

func newTransport(t *testing.T, tokenHandler http.HandlerFunc, store secrets.Store, clk clock.Clock) (*Transport, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokenHandler(w, r)
			return
		}
		w.Header().Set("X-Seen-Auth", r.Header.Get("Authorization"))
	}))
	t.Cleanup(srv.Close)
	return New(Config{
		OAuth: &oauth2.Config{
			ClientID: "id", ClientSecret: "secret",
			Endpoint: oauth2.Endpoint{TokenURL: srv.URL + "/token", AuthStyle: oauth2.AuthStyleInParams},
		},
		Store: store, AccountID: "a1", RefreshToken: "rt-1", Clock: clk,
	}), srv
}

func get(t *testing.T, tr *Transport, url string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	require.NoError(t, err)
	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.Header.Get("X-Seen-Auth")
}

func TestRefreshesOnExpiryAndInvalidate(t *testing.T) {
	var calls atomic.Int32
	clk := clock.NewFake(time.Now())
	tr, srv := newTransport(t, func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-` + string(rune('0'+n)) + `","token_type":"Bearer","expires_in":3600}`))
	}, secrets.NewMemory(), clk)

	assert.Equal(t, "Bearer at-1", get(t, tr, srv.URL+"/x"))
	assert.Equal(t, "Bearer at-1", get(t, tr, srv.URL+"/x"), "期限内はキャッシュ")

	tr.Invalidate()
	assert.Equal(t, "Bearer at-2", get(t, tr, srv.URL+"/x"))

	clk.Advance(2 * time.Hour)
	assert.Equal(t, "Bearer at-3", get(t, tr, srv.URL+"/x"), "期限切れで更新")
}

func TestPersistsRotatedRefreshToken(t *testing.T) {
	store := secrets.NewMemory()
	tr, srv := newTransport(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-2"}`))
	}, store, clock.NewFake(time.Now()))
	get(t, tr, srv.URL+"/x")
	got, err := store.Get(secrets.RefreshTokenKey("a1"))
	require.NoError(t, err)
	assert.Equal(t, "rt-2", got)
}

func TestRefreshFailureIsMarkedAndKeepsCause(t *testing.T) {
	tr, srv := newTransport(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}, secrets.NewMemory(), clock.NewFake(time.Now()))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/x", http.NoBody)
	require.NoError(t, err)

	_, err = tr.RoundTrip(req) //nolint:bodyclose // エラー時は応答が無い
	require.ErrorIs(t, err, ErrRefresh, "リクエストを送る前の失敗と分かる")
	var re *oauth2.RetrieveError
	assert.True(t, errors.As(err, &re), "元の更新エラーも辿れる")
}
