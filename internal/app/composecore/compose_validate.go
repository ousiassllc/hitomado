package composecore

import (
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/confirm"
	"github.com/ousiassllc/hitomado/internal/domain/contacts"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/senderpolicy"
)

// normalizeAddrs は空のアドレスを除き、メールアドレスを小文字・前後空白除去、表示名を前後空白除去する。
func normalizeAddrs(in []model.Address) []model.Address {
	out := make([]model.Address, 0, len(in))
	for _, a := range in {
		email := contacts.Normalize(a.Email)
		if email == "" {
			continue
		}
		out = append(out, model.Address{Name: strings.TrimSpace(a.Name), Email: email})
	}
	return out
}

// normalizeContent は PrepareSend／Send／SaveDraft が同じ形で扱えるよう入力内容を正規化する。
func normalizeContent(c dto.ComposeContent) dto.ComposeContent {
	return dto.ComposeContent{
		To: normalizeAddrs(c.To), Cc: normalizeAddrs(c.Cc), Bcc: normalizeAddrs(c.Bcc),
		Subject: c.Subject, BodyHTML: c.BodyHTML,
	}
}

func allRecipients(c dto.ComposeContent) []model.Address {
	out := make([]model.Address, 0, len(c.To)+len(c.Cc)+len(c.Bcc))
	out = append(out, c.To...)
	out = append(out, c.Cc...)
	return append(out, c.Bcc...)
}

// validateSend は送信前検証（API §5.2 の順序）: 送信元→宛先→アドレス形式→送信元整合→添付サイズ。
// c は正規化済みであること。
func validateSend(cs composeSession, c dto.ComposeContent) error {
	if cs.accountID == "" {
		return apperr.New(apperr.SenderRequired, "送信元アカウントを選択してください。")
	}
	rcpts := allRecipients(c)
	if len(rcpts) == 0 {
		return apperr.New(apperr.NoRecipients, "宛先を 1 件以上入力してください。")
	}
	var bad []string
	for _, a := range rcpts {
		if !contacts.IsValid(a.Email) {
			bad = append(bad, a.Email)
		}
	}
	if len(bad) > 0 {
		return apperr.New(apperr.InvalidAddress, "メールアドレスの形式が正しくありません。").WithDetail("emails", bad)
	}
	if err := checkSenderPolicy(cs); err != nil {
		return err
	}
	if cs.attachmentsTotal() > attachmentLimitBytes {
		return errAttachmentTooLarge()
	}
	return nil
}

// checkSenderPolicy は reply／forward のとき、送信元がスレッドの所有アカウントと一致することを検査する（NF-S11）。
func checkSenderPolicy(cs composeSession) error {
	if cs.thread == nil {
		return nil
	}
	if err := senderpolicy.Check(cs.accountID, cs.threadAccountID); err != nil {
		return apperr.Wrap(apperr.SenderMismatch, "送信元と受信アカウントが一致しません。", err)
	}
	return nil
}

// contentHash は確認トークンに紐づける内容ハッシュ。c は正規化済みであること。
func contentHash(cs composeSession, c dto.ComposeContent) confirm.Hash {
	refs := make([]confirm.AttachmentRef, 0, len(cs.attachments))
	for _, a := range cs.attachments {
		refs = append(refs, confirm.AttachmentRef{ID: a.ID, Size: a.Size})
	}
	return confirm.HashContent(confirm.ContentInput{
		AccountID: cs.accountID, To: c.To, Cc: c.Cc, Bcc: c.Bcc,
		Subject: c.Subject, BodyHTML: c.BodyHTML, Attachments: refs, Archive: cs.archive,
	})
}
