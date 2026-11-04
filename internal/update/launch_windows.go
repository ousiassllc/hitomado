//go:build windows

package update

import (
	"fmt"
	"os/exec"
	"syscall"
)

// AutoInstallSupported はこのビルドが更新ファイルの入れ替えを行えるか（Windows のみ。NF-D03）。
const AutoInstallSupported = true

// Windows のプロセス作成フラグ。
const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// NewLauncher は NSIS インストーラーをサイレントで、親（このアプリ）が終了しても生き続ける別プロセスとして起動する。
func NewLauncher() Launcher {
	return func(installer string, opts LaunchOptions) error {
		cmd := exec.Command(installer, installerArgs(opts)...) //nolint:gosec // 照合済みの更新ファイルのパスと固定の引数
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: detachedProcess | createNewProcessGroup,
			HideWindow:    true,
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start installer: %w", err)
		}
		// 完了は待たない。ハンドルだけ手放す。
		return cmd.Process.Release()
	}
}
