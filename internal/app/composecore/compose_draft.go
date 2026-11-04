package composecore

import (
	"context"
	"errors"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/contacts"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

// SaveDraft は Gmail の下書きを作成・更新する（F-12）。送信元未選択は SENDER_REQUIRED。
// 添付（ローカル・転送元・復元した下書きの添付）も下書きに含める（F-12）。
// 宛先は形式が正しいアドレスだけを保存する（入力途中の値で保存を止めない）。
func (s *Service) SaveDraft(ctx context.Context, sessionID string, content dto.ComposeContent) (dto.SaveDraftResult, error) {
	defer s.d.Busy.Begin()() // 保存中は自動アップデートの入れ替えをしない
	cs, view, err := s.snapshot(sessionID)
	if err != nil {
		return dto.SaveDraftResult{}, err
	}
	if view.accountID == "" {
		return dto.SaveDraftResult{}, apperr.New(apperr.SenderRequired, "送信元アカウントを選択してください。")
	}
	if !cs.io.lock(ctx.Done()) {
		return dto.SaveDraftResult{}, apperr.FromInfra(ctx.Err(), "")
	}
	defer cs.io.unlock()
	// ロック待ちの間に別の保存・送信が進んだ可能性があるので、最新の状態を読み直す。
	if _, view, err = s.snapshot(sessionID); err != nil {
		return dto.SaveDraftResult{}, err
	}

	c := normalizeContent(content)
	acct, err := s.account(ctx, view.accountID)
	if err != nil {
		return dto.SaveDraftResult{}, err
	}
	atts, fetched, err := s.outgoingAttachments(ctx, view)
	if err != nil {
		return dto.SaveDraftResult{}, err
	}
	raw, err := gmail.BuildDraftMIME(model.OutgoingMessage{
		From: model.Address{Name: foldLine(acct.DisplayName), Email: acct.Email},
		To:   validAddrs(c.To), Cc: validAddrs(c.Cc), Bcc: validAddrs(c.Bcc),
		Subject: c.Subject, BodyHTML: c.BodyHTML,
		InReplyTo: view.inReplyTo, References: view.references,
		Attachments: atts, Date: s.d.Clock.Now(),
	})
	if err != nil {
		return dto.SaveDraftResult{}, apperr.Wrap(apperr.Validation, "下書きを作成できません。内容を確認してください。", err)
	}

	draftID, err := s.writeDraft(ctx, view, raw)
	if err != nil {
		return dto.SaveDraftResult{}, apperr.FromInfra(err, view.accountID)
	}
	s.keepFetched(sessionID, fetched)
	if draftID != view.draftID || view.draftAcctID != view.accountID {
		s.discardStaleDraft(ctx, view)
		// 下書きの作成を反映する。スレッド本文のキャッシュに DRAFT メッセージが無いと、開いたときに下書きの導線が出ない。
		s.invalidateThread(view.accountID, view.sendThreadID)
	}

	s.mu.Lock()
	if live, ok := s.sessions[sessionID]; ok {
		live.draftID, live.draftAcctID, live.lastContent = draftID, view.accountID, c
	}
	s.mu.Unlock()
	return dto.SaveDraftResult{DraftID: draftID, SavedAt: s.d.Clock.Now().UnixMilli()}, nil
}

// invalidateThread は当該スレッドの本文キャッシュを破棄する（threadID が空なら何もしない）。
func (s *Service) invalidateThread(accountID, threadID string) {
	if threadID != "" {
		s.d.Threads.Invalidate(accountID, threadID)
	}
}

// writeDraft は既存の下書きを更新し、無い（または別アカウントの）ときは新規作成する。
// 下書きが Gmail 側で削除されていた（NotFound）ときも新規作成する。
func (s *Service) writeDraft(ctx context.Context, view composeSession, raw []byte) (string, error) {
	if view.draftID != "" && view.draftAcctID == view.accountID {
		err := s.d.Gmail.UpdateDraft(ctx, view.accountID, view.draftID, raw)
		if err == nil {
			return view.draftID, nil
		}
		if !errors.Is(err, gmail.ErrNotFound) {
			return "", err
		}
	}
	return s.d.Gmail.CreateDraft(ctx, view.accountID, raw, view.sendThreadID)
}

// discardStaleDraft は送信元の切り替えで置き去りになった旧アカウントの下書きを best-effort で削除する。
func (s *Service) discardStaleDraft(ctx context.Context, view composeSession) {
	if view.draftID == "" || view.draftAcctID == view.accountID {
		return
	}
	if err := s.d.Gmail.DeleteDraft(ctx, view.draftAcctID, view.draftID); err != nil && !errors.Is(err, gmail.ErrNotFound) {
		s.log().Warn("旧アカウントの下書きを削除できません", "accountId", view.draftAcctID, "err", err)
	}
}

func validAddrs(in []model.Address) []model.Address {
	out := make([]model.Address, 0, len(in))
	for _, a := range in {
		if contacts.IsValid(a.Email) {
			out = append(out, a)
		}
	}
	return out
}
