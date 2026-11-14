//go:build cgo && !netgo

package androidnet

// cgoResolver は Go の標準の解決器の既定（Android・cgo では bionic の getaddrinfo。net/conf.go の goosPrefersCgo）。
const cgoResolver = "cgo"
