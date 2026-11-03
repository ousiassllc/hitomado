//go:build android && !selfcheck

package main

// startAndroidSelfCheck は検証用の自己診断（androidselfcheck_android.go）を `-tags selfcheck` のときだけ有効にするための空実装。
func startAndroidSelfCheck(string) {}
