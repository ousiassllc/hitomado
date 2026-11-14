package androidnet

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/hitomado/internal/exportfile"
)

func TestParseDiagnostics(t *testing.T) {
	d, err := ParseDiagnostics(`{"process":"cached","network":"vpn","validated":true,"private_dns":"off","dns_servers":["10.0.0.1","fe80::1"],"device_idle":true}`)
	require.NoError(t, err)
	assert.Equal(t, "cached", d.Process)
	assert.True(t, d.DeviceIdle)
	assert.Equal(t, []string{"10.0.0.1", "fe80::1"}, d.DNSServers)
	_, err = ParseDiagnostics("not json")
	assert.Error(t, err)
}

func TestDiagnosticsAttrs_OnlyIPsAsDNSServers(t *testing.T) {
	attrs := Diagnostics{DNSServers: []string{"10.0.0.1", "dns.example", "fe80::1"}}.Attrs()
	got := fmt.Sprint(attrs...)
	assert.Contains(t, got, "10.0.0.1,fe80::1")
	assert.NotContains(t, got, "dns.example")
}

func TestResolverKind(t *testing.T) {
	assert.Equal(t, "go", ResolverKind(true, ""))
	assert.Equal(t, cgoResolver, ResolverKind(false, ""))
	assert.Equal(t, cgoResolver+" (GODEBUG netdns=go)", ResolverKind(false, "x=1, netdns=go"))
}

func TestMapResolveError(t *testing.T) {
	bg := mapResolveError(&exportfile.BridgeError{Code: "background_no_network"})
	assert.ErrorIs(t, bg, ErrBackgroundBlocked)
	for _, code := range []string{"no_network", "unresolved", "failed", "bad_request"} {
		assert.NotErrorIs(t, mapResolveError(&exportfile.BridgeError{Code: code}), ErrBackgroundBlocked, code)
	}
	plain := errors.New("x")
	assert.Same(t, plain, mapResolveError(plain))
}
