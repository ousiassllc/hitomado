package update

import "errors"

// 画面に出す文面（URL・パスなど機密になり得るものを含めない）。
const (
	interruptedMessage = "前回の更新の入れ替えが完了しませんでした。「再起動して更新」でもう一度試せます。"
	checksumMessage    = "ダウンロードしたファイルが、公開されているハッシュと一致しなかったため使わずに破棄しました。今のバージョンのまま使えます。"
	busyMessage        = "送信中または下書きの保存中のため、いまは入れ替えられません。"
	launchMessage      = "インストーラーを起動できませんでした。今のバージョンのまま使えます。"
	downloadMessage    = "更新ファイルをダウンロードできませんでした。今のバージョンのまま使えます。次の確認でやり直します。"
)

// UserMessage は err を、画面に出してよい日本語の文面にする（URL・パスなどを含めない）。
func UserMessage(err error) string {
	switch {
	case errors.Is(err, ErrChecksumMismatch):
		return checksumMessage
	case errors.Is(err, ErrBusy):
		return busyMessage
	case errors.Is(err, ErrUnsupported), errors.Is(err, errLaunch):
		return launchMessage
	default:
		return downloadMessage
	}
}
