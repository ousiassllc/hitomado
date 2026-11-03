//go:build android

package main

import (
	"github.com/ousiassllc/hitomado/internal/oauth"
	"github.com/ousiassllc/hitomado/internal/secrets"
)

// newOAuthTracker は Android で、認可の途中でプロセスが止められたことを検出する Tracker を返す。
// Android はブラウザに出ている間のアプリのプロセスを終了させうる。127.0.0.1 の待受はプロセスと一緒に消えるため、
// その認可は完了できず、次の起動で「ログインはやり直し」と案内する（docs/environment/android.md §14）。
func newOAuthTracker(kv secrets.Store) *oauth.Tracker { return oauth.NewTracker(kv) }
