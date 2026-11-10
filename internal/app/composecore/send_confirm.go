package composecore

import (
	"context"
	"strconv"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/store"
)

// SendConfirmEnabled は送信前に確認画面を出す設定（既定オン。#313）。未設定・壊れた値は有効。
// 確認画面を省くのはフロントの送信フローだけで、確認トークンの検証（Send）は設定にかかわらず行う。
func SendConfirmEnabled(ctx context.Context, d *deps.Deps) (bool, error) {
	raw, ok, err := d.Store.GetSetting(ctx, store.SettingSendConfirm)
	if err != nil {
		return false, apperr.FromInfra(err, "")
	}
	if !ok {
		return true, nil
	}
	v, perr := strconv.ParseBool(raw)
	if perr != nil {
		return true, nil
	}
	return v, nil
}

// SetSendConfirmEnabled は送信前に確認画面を出すか（既定オン）を保存する。
func SetSendConfirmEnabled(ctx context.Context, d *deps.Deps, enabled bool) error {
	if err := d.Store.SetSetting(ctx, store.SettingSendConfirm, strconv.FormatBool(enabled)); err != nil {
		return apperr.FromInfra(err, "")
	}
	return nil
}
