//go:build !android

package main

import (
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

// newOAuthTracker はデスクトップでは検出しない（nil）。Windows の認可の動きは変えない。
func newOAuthTracker(secrets.Store) *oauth.Tracker { return nil }
