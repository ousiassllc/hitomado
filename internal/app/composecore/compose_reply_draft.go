package composecore

import (
	"context"
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/bodyhtml"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
)

// CreateReplyDraft は ref のスレッドへの返信（reply）の Gmail 下書きを、画面を介さずに作る（エージェントモード。#372）。
// 宛先・件名・In-Reply-To／References・署名・引用は、利用者が「返信」を開いたときと同じ（OpenCompose の reply）で、
// bodyText（プレーンテキストの本文）は、AI 下書きと同じく署名・引用の前に入れる。署名は受信したアカウントのもの。
// 作成セッションは作ったあとで必ず閉じる。送信はしない（下書きを作るだけで、送信系は呼ばない）。
// 戻り値は Gmail の下書き ID と、下書きの宛先。
func (s *Service) CreateReplyDraft(ctx context.Context, ref dto.ThreadRef, bodyText string) (draftID string, to []model.Address, err error) {
	body := bodyhtml.TextToHTML(bodyText)
	if body == "" {
		return "", nil, apperr.New(apperr.Validation, "返信の本文が空です。")
	}
	session, err := s.OpenCompose(ctx, dto.OpenComposeRequest{Mode: string(model.ModeReply), Ref: &ref})
	if err != nil {
		return "", nil, err
	}
	defer s.drop(session.SessionID)

	content := session.Content
	content.BodyHTML = body + strings.TrimSpace(content.BodyHTML)
	saved, err := s.SaveDraft(ctx, session.SessionID, content)
	if err != nil {
		return "", nil, err
	}
	return saved.DraftID, content.To, nil
}
