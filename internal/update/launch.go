package update

import "strconv"

// installerWait は、起動したインストーラーが hitomado.exe のロックの解除（アプリの終了）を待つ秒数の上限。
// インストーラーはアプリの終了前に起動され得るため、サイレント実行でも実行中の上書き拒否（終了コード 5）の前に待つ
// （build/windows/nsis/project.nsi の /WAIT=秒）。
const installerWait = 60

// installerArgs は Windows の NSIS インストーラーへ渡す引数。
//
//	/S         サイレント（画面なし）。管理者権限は要らない（利用者ごとのインストール）
//	/WAIT=60   アプリの終了を最大 60 秒待ってから上書きする
//	/RELAUNCH  インストール完了後にアプリを起動し直す（「再起動して更新」のとき）
//
// /D= は渡さない（前回のインストール先をインストーラーが引き継ぐ）。
func installerArgs(opts LaunchOptions) []string {
	args := []string{"/S", "/WAIT=" + strconv.Itoa(installerWait)}
	if opts.Relaunch {
		args = append(args, "/RELAUNCH")
	}
	return args
}
