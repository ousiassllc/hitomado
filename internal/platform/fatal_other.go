//go:build !windows

package platform

// ShowFatal は Windows 以外ではダイアログを出さない（標準エラーとログに出る）。
func ShowFatal(string, string) {}
