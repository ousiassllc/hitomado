//go:build !cgo || netgo

package androidnet

// cgoResolver は cgo が無い（または netgo）ビルドでは Go の DNS クライアント（Android では /etc/resolv.conf が無く、127.0.0.1:53 を引く）。
const cgoResolver = "go"
