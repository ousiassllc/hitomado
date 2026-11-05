package composecore

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/bodyhtml"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/confirm"
	"github.com/ousiassllc/hitomado/internal/domain/contacts"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// bodyPreviewRunes は SendPreview.BodyPreview の最大文字数（改行を含む）。
const bodyPreviewRunes = 600

// labelInbox は受信箱のシステムラベル。
const labelInbox = "INBOX"

// PrepareSend は送信前検証を行い、確認ダイアログ用の内容と確認トークン（5 分・1 回限り）を返す（API §5.2）。
// 再実行すると以前のトークンは無効になる。
func (s *Service) PrepareSend(ctx context.Context, sessionID string, content dto.ComposeContent, options dto.SendOptions) (dto.SendPreview, error) {
	cs, view, err := s.snapshot(sessionID)
	if err != nil {
		return dto.SendPreview{}, err
	}
	c := normalizeContent(content)
	if err := validateSend(view, c); err != nil {
		return dto.SendPreview{}, err
	}
	acct, err := s.account(ctx, view.accountID)
	if err != nil {
		return dto.SendPreview{}, err
	}
	warnings, err := s.warnings(ctx, acct, c)
	if err != nil {
		return dto.SendPreview{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if live, ok := s.sessions[sessionID]; !ok || live != cs {
		return dto.SendPreview{}, errSessionNotFound()
	} else if live.sending {
		return dto.SendPreview{}, errSessionBusy()
	}
	// 添付・送信元は検証中に変わり得るので、ロック下の最新状態でハッシュを取る。
	cs.archive = options.ArchiveAfterSend
	cs.lastContent = c
	if err := validateSend(*cs, c); err != nil {
		return dto.SendPreview{}, err
	}
	tok, exp := s.confirm.Issue(sessionID, contentHash(*cs, c))
	return dto.SendPreview{
		ConfirmToken:    string(tok),
		ExpiresAt:       exp.UnixMilli(),
		From:            dto.PreviewFrom{Address: model.Address{Name: foldLine(acct.DisplayName), Email: acct.Email}, AccountID: acct.ID, Color: acct.Color},
		To:              c.To,
		Cc:              c.Cc,
		Bcc:             c.Bcc,
		Subject:         c.Subject,
		AttachmentCount: len(cs.attachments),
		BodyPreview:     textconv.Preview(bodyhtml.UserBodyHTML(c.BodyHTML), bodyPreviewRunes),
		Warnings:        warnings,
	}, nil
}

// warnings は送信前の警告（エラーにはしない）。
func (s *Service) warnings(ctx context.Context, sender store.Account, c dto.ComposeContent) ([]dto.SendWarning, error) {
	out := []dto.SendWarning{}
	accts, err := s.d.Store.ListAccounts(ctx)
	if err != nil {
		return nil, apperr.FromInfra(err, "")
	}
	own := map[string]bool{}
	for _, a := range accts {
		if a.ID != sender.ID {
			own[contacts.Normalize(a.Email)] = true
		}
	}
	var others []string
	seen := map[string]bool{}
	for _, a := range allRecipients(c) {
		if own[a.Email] && !seen[a.Email] {
			seen[a.Email] = true
			others = append(others, a.Email)
		}
	}
	if len(others) > 0 {
		out = append(out, dto.SendWarning{Code: "OTHER_OWN_ACCOUNT", Message: "宛先に、他の自分のアカウントのアドレスが含まれています。", Emails: others})
	}
	if strings.TrimSpace(c.Subject) == "" {
		out = append(out, dto.SendWarning{Code: "EMPTY_SUBJECT", Message: "件名が空です。"})
	}
	if bodyhtml.IsEmpty(c.BodyHTML) {
		out = append(out, dto.SendWarning{Code: "EMPTY_BODY", Message: "本文が空です。"})
	}
	return out, nil
}

// Send は確認トークンを検証して送信する。トークンは成功・失敗にかかわらず 1 回の Send で使用済みになる
// （ハッシュ不一致のときだけは消費されず、内容を戻せば再送できる）。同一セッションの Send は排他。
func (s *Service) Send(ctx context.Context, sessionID, confirmToken string, content dto.ComposeContent) (dto.SendResult, error) {
	defer s.d.Busy.Begin()() // 送信中は自動アップデートの入れ替えをしない
	cs, view, err := s.beginSend(sessionID, confirmToken, content)
	if err != nil {
		return dto.SendResult{}, err
	}
	// 以降は成功・失敗にかかわらずセッションの sending を戻す。成功時は破棄する。
	sent := false
	defer func() { s.endSend(cs, sent) }()

	c := normalizeContent(content)
	acct, err := s.account(ctx, view.accountID)
	if err != nil {
		return dto.SendResult{}, err
	}
	atts, _, err := s.outgoingAttachments(ctx, view)
	if err != nil {
		return dto.SendResult{}, err
	}
	raw, err := gmail.BuildMIME(model.OutgoingMessage{
		From: model.Address{Name: foldLine(acct.DisplayName), Email: acct.Email},
		To:   c.To, Cc: c.Cc, Bcc: c.Bcc,
		Subject: c.Subject, BodyHTML: bodyhtml.StripMarkers(c.BodyHTML), // 宛先に内部マーカーを届けない
		InReplyTo: view.inReplyTo, References: view.references,
		Attachments: atts, Date: s.d.Clock.Now(),
	})
	if err != nil {
		return dto.SendResult{}, apperr.Wrap(apperr.Validation, "メールを組み立てられません。宛先・件名を確認してください。", err)
	}
	if !cs.io.lock(ctx.Done()) {
		return dto.SendResult{}, apperr.FromInfra(ctx.Err(), view.accountID)
	}
	defer cs.io.unlock()
	// ロック待ちの間に SaveDraft が下書きを作成・更新した可能性があるので、最新の値を読み直す
	// （古い値のままだと送信済みの下書きが残る）。
	// 同じ区間で、待機中に DiscardCompose などでセッションが破棄されていないことも確かめる
	// （破棄済みなら送信しない。消費済みの確認トークンはそのまま）。
	s.mu.Lock()
	live := s.sessions[sessionID] == cs
	view.draftID, view.draftAcctID = cs.draftID, cs.draftAcctID
	s.mu.Unlock()
	if !live {
		return dto.SendResult{}, errSessionNotFound()
	}

	msg, err := s.d.Gmail.Send(ctx, view.accountID, raw, view.sendThreadID)
	if err != nil {
		return dto.SendResult{}, apperr.FromInfra(err, view.accountID)
	}
	sent = true
	res := dto.SendResult{MessageID: msg.MessageID, ThreadID: msg.ThreadID}
	s.afterSend(ctx, view, c, msg, &res)
	return res, nil
}

// beginSend はトークン・送信元整合を検証して sending を立てる。確認ハッシュは PrepareSend 時の archive を含めて取る。
func (s *Service) beginSend(sessionID, token string, content dto.ComposeContent) (*composeSession, composeSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cs, err := s.getLocked(sessionID)
	if err != nil {
		return nil, composeSession{}, err
	}
	if cs.sending {
		return nil, composeSession{}, errSessionBusy()
	}
	c := normalizeContent(content)
	if err := validateSend(*cs, c); err != nil {
		return nil, composeSession{}, err
	}
	if err := s.confirm.Verify(sessionID, confirm.Token(token), contentHash(*cs, c)); err != nil {
		return nil, composeSession{}, confirmError(err)
	}
	cs.sending = true
	return cs, cs.clone(), nil
}

func confirmError(err error) error {
	switch {
	case errors.Is(err, confirm.ErrExpired):
		return apperr.Wrap(apperr.ConfirmExpired, "送信確認の有効期限が切れています。もう一度確認してください。", err)
	case errors.Is(err, confirm.ErrMismatch):
		return apperr.Wrap(apperr.ConfirmMismatch, "確認後に内容が変更されました。もう一度確認してください。", err)
	default:
		return apperr.Wrap(apperr.ConfirmRequired, "送信前の確認が必要です。", err)
	}
}

// endSend は sending を戻し、送信成功ならセッションを破棄する。
func (s *Service) endSend(cs *composeSession, sent bool) {
	s.mu.Lock()
	cs.sending = false
	cs.lastUsed = s.d.Clock.Now()
	if sent {
		delete(s.sessions, cs.id)
	}
	s.mu.Unlock()
	if sent {
		s.confirm.Invalidate(cs.id)
	}
}

// afterSend は送信成功後の best-effort 処理（アーカイブ・下書き削除・連絡先加算・キャッシュ更新）。
// どれが失敗しても送信は成功として扱う。
func (s *Service) afterSend(ctx context.Context, view composeSession, c dto.ComposeContent, msg gmail.SentMessage, res *dto.SendResult) {
	acctID := view.accountID
	// 呼び出し側 ctx が取り消されても送信後の後始末は続ける。
	ctx = context.WithoutCancel(ctx)

	// アーカイブ・キャッシュ更新の対象は元のスレッド（転送でも元スレッド。転送の送信先は別スレッド）。
	var origThreadID string
	if view.thread != nil {
		origThreadID = view.thread.ThreadID
	}
	if view.archive && origThreadID != "" {
		if err := s.d.Gmail.ModifyThread(ctx, acctID, origThreadID, nil, []string{labelInbox}); err != nil {
			res.ArchiveError = apperr.ToMap(apperr.FromInfra(err, acctID))
			s.log().Warn("送信後のアーカイブに失敗しました", "accountId", acctID)
		} else {
			res.Archived = true
		}
	}
	if view.draftID != "" {
		if err := s.d.Gmail.DeleteDraft(ctx, view.draftAcctID, view.draftID); err != nil && !errors.Is(err, gmail.ErrNotFound) {
			s.log().Warn("送信後の下書き削除に失敗しました", "accountId", view.draftAcctID)
		}
	}
	if err := s.d.Store.AddSentContacts(ctx, acctID, allRecipients(c), s.d.Clock.Now()); err != nil {
		s.log().Warn("宛先候補の加算に失敗しました", "accountId", acctID)
	}
	for _, id := range uniqueIDs(origThreadID, msg.ThreadID) {
		s.d.Threads.Invalidate(acctID, id)
		if s.d.Sync != nil {
			if _, _, err := s.d.Sync.RefreshThread(ctx, acctID, id); err != nil {
				s.log().Warn("送信後のスレッド更新に失敗しました", "accountId", acctID)
			}
		}
	}
}

// uniqueIDs は空でない ID を重複なく返す。
func uniqueIDs(ids ...string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
