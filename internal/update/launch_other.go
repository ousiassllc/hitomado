//go:build !windows

package update

// AutoInstallSupported はこのビルドが更新ファイルの入れ替えを行えるか（Windows のみ。Android・開発用の Linux は行わない）。
const AutoInstallSupported = false

// NewLauncher は ErrUnsupported を返す Launcher（Windows 以外では入れ替えない）。
func NewLauncher() Launcher {
	return func(string, LaunchOptions) error { return ErrUnsupported }
}
