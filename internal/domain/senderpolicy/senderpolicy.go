// Package senderpolicy は送信元の整合検証を提供する（NF-S11）。
// From は常にアカウントの登録アドレスから決まり、受信アカウント以外からの返信・転送は拒否する。
package senderpolicy

import (
	"errors"

	"github.com/ousiassllc/hitomado/internal/domain/model"
)

var (
	// ErrSenderMismatch はセッションのアカウントがスレッドの所有アカウントと異なることを表す。
	ErrSenderMismatch = errors.New("送信元アカウントがスレッドの受信アカウントと一致しません")
	// ErrSenderLocked は返信・転送で送信元を変更しようとしたことを表す。
	ErrSenderLocked = errors.New("返信・転送では送信元を変更できません")
)

// Check はセッションのアカウントがスレッドの所有アカウントと一致することを検証する。
func Check(sessionAccountID, threadAccountID string) error {
	if sessionAccountID != threadAccountID {
		return ErrSenderMismatch
	}
	return nil
}

// CanChangeSender は mode で送信元を変更できるかを返す。新規作成以外は ErrSenderLocked。
func CanChangeSender(mode model.Mode) error {
	if mode != model.ModeNew {
		return ErrSenderLocked
	}
	return nil
}
