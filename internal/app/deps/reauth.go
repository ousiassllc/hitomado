package deps

import (
	"context"
	"log/slog"

	"github.com/ousiassllc/hitomado/internal/events"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/store"
)

// Log は Logger（未設定なら slog の既定）。
func (d *Deps) Log() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// MarkNeedsReauth は利用者の操作（スレッドを開く・送信など）の失敗でアカウントを要再接続にし、account:status を発行する。
// cause は失敗した元のエラーで、理由（gmail.Reason*）をログに残すために使う（同期周期の経路は internal/sync が同じ形で残す）。
func (d *Deps) MarkNeedsReauth(ctx context.Context, accountID, message string, cause error) {
	ctx = context.WithoutCancel(ctx)
	acc, err := d.Store.GetAccount(ctx, accountID)
	if err != nil || acc.Status == store.StatusNeedsReauth {
		return
	}
	d.Log().Warn("account needs reauth", "account", accountID, "reason", gmail.ReauthReason(cause), "trigger", "action",
		logging.OriginKey, logging.OriginApp, "prev_status", acc.Status, "err", cause)
	if err := d.Store.SetAccountStatus(ctx, accountID, store.StatusNeedsReauth, message); err != nil {
		d.Log().Warn("アカウント状態の更新に失敗", "accountId", accountID, "err", err)
		return
	}
	d.Events.Emit(events.AccountStatus, events.AccountStatusPayload{AccountID: accountID, Status: store.StatusNeedsReauth, Message: message})
}
