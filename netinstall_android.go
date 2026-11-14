//go:build android

package main

// 名前解決の救済・記録（#368）。インポートすると、init で http.DefaultTransport の接続が差し替わる（internal/androidnet）。
import _ "github.com/ousiassllc/hitomado/internal/androidnet"
