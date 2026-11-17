//go:build !windows

package platform

import "github.com/wailsapp/wails/v3/pkg/application"

// KeepWindowIcon は Windows 以外では何もしない（タスクバーのアイコンの問題は Windows 固有）。
func KeepWindowIcon(*application.WebviewWindow) {}
