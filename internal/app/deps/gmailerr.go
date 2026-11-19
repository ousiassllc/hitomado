package deps

import (
	"context"
	"slices"

	"github.com/ousiassllc/nefos-inbox/internal/app/apperr"
	"github.com/ousiassllc/nefos-inbox/internal/gmail"
	syncer "github.com/ousiassllc/nefos-inbox/internal/sync"
)

// GmailErr は Gmail（とその周辺）のエラーを AppError にする。NEEDS_REAUTH はアカウントを要再接続にして
// account:status を発行する（バインディング仕様 §10）。err は nil でないこと。
func (d *Deps) GmailErr(ctx context.Context, accountID string, err error) *apperr.Error {
	ae := apperr.FromInfra(err, accountID)
	if ae.Code == apperr.NeedsReauth && accountID != "" {
		message := ae.Message
		if gmail.ReauthReason(err) == gmail.ReasonInvalidGrant {
			message = syncer.MsgTokenRevoked // 同期周期の経路と同じメッセージにして、画面が原因の案内を出せるようにする
		}
		d.MarkNeedsReauth(ctx, accountID, message, err)
	}
	return ae
}

// FindThreadDraft はスレッドの下書き ID を返す（無ければ空文字）。DRAFT ラベルのメッセージが無ければ下書きは無いので、
// drafts.list の全ページ走査を避ける。検索に失敗してもスレッドは読めるよう、ctx の取り消し以外のエラーは警告に留める。
func (d *Deps) FindThreadDraft(ctx context.Context, accountID, threadID string, msgs []gmail.MessageFull) (string, error) {
	if !slices.ContainsFunc(msgs, func(m gmail.MessageFull) bool { return slices.Contains(m.LabelIDs, "DRAFT") }) {
		return "", nil
	}
	draftID, ok, err := d.Gmail.FindDraftForThread(ctx, accountID, threadID)
	switch {
	case err != nil && ctx.Err() != nil:
		return "", apperr.FromInfra(ctx.Err(), accountID)
	case err != nil:
		d.Log().Warn("スレッドの下書き検索に失敗", "accountId", accountID, "err", err)
	case ok:
		return draftID, nil
	}
	return "", nil
}
