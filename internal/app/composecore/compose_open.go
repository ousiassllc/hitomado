package composecore

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/bodyhtml"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/quote"
	"github.com/ousiassllc/hitomado/internal/domain/reply"
	"github.com/ousiassllc/hitomado/internal/domain/signature"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// labelDraft は Gmail の下書きメッセージに付くシステムラベル。
const labelDraft = "DRAFT"

// OpenCompose は作成セッションを作り、宛先・件名・本文（署名・引用込み）の初期値を返す（API §5.1）。
// draftId 指定時は Gmail 下書きの内容で上書きする。From は accounts.email のみから決める。
func (s *Service) OpenCompose(ctx context.Context, req dto.OpenComposeRequest) (dto.ComposeSession, error) {
	cs := &composeSession{id: store.NewID(), mode: model.Mode(req.Mode), io: newChanMutex()}
	var (
		content dto.ComposeContent
		attErr  error
		err     error
	)
	switch cs.mode {
	case model.ModeNew:
		content, err = s.initNew(ctx, cs, req)
	case model.ModeReply, model.ModeReplyAll, model.ModeForward:
		content, err = s.initThreaded(ctx, cs, req)
	default:
		err = apperr.New(apperr.Validation, "作成の種別が正しくありません。")
	}
	if err != nil {
		return dto.ComposeSession{}, err
	}
	if req.DraftID != "" {
		if cs.accountID == "" {
			return dto.ComposeSession{}, apperr.New(apperr.Validation, "下書きを開くには送信元アカウントが必要です。")
		}
		if content, attErr, err = s.restoreDraft(ctx, cs, req.DraftID); err != nil {
			return dto.ComposeSession{}, err
		}
	}
	cs.lastContent = content
	cs.lastUsed = s.d.Clock.Now()

	s.mu.Lock()
	s.expireLocked()
	s.sessions[cs.id] = cs
	out := cs.toDTO(content)
	s.mu.Unlock()
	if attErr != nil {
		out.AttachmentError = apperr.ToMap(attErr)
	}
	return out, nil
}

func (cs *composeSession) setSender(a store.Account) {
	cs.accountID = a.ID
	cs.from = model.Address{Name: foldLine(a.DisplayName), Email: a.Email}
}

func emptyContent() dto.ComposeContent {
	return dto.ComposeContent{To: []model.Address{}, Cc: []model.Address{}, Bcc: []model.Address{}}
}

// initNew は新規作成の初期値。accountId があればそのアカウント（署名挿入）、無ければ送信元未選択。
func (s *Service) initNew(ctx context.Context, cs *composeSession, req dto.OpenComposeRequest) (dto.ComposeContent, error) {
	content := emptyContent()
	if req.AccountID == "" {
		return content, nil
	}
	acct, err := s.account(ctx, req.AccountID)
	if err != nil {
		return content, err
	}
	cs.setSender(acct)
	content.BodyHTML = signature.Insert("", acct.ID, acct.SignatureHTML)
	return content, nil
}

// initThreaded は reply／reply_all／forward の初期値。
func (s *Service) initThreaded(ctx context.Context, cs *composeSession, req dto.OpenComposeRequest) (dto.ComposeContent, error) {
	ref := req.Ref
	if ref == nil || ref.AccountID == "" || ref.ThreadID == "" {
		return emptyContent(), apperr.New(apperr.Validation, "返信・転送にはスレッドの指定が必要です。")
	}
	acct, err := s.account(ctx, ref.AccountID)
	if err != nil {
		return emptyContent(), err
	}
	thread, err := s.d.Threads.Get(ctx, ref.AccountID, ref.ThreadID)
	if err != nil {
		return emptyContent(), apperr.FromInfra(err, ref.AccountID)
	}
	msg, ok := pickSource(thread.Messages, acct.Email, req.SourceMessageID)
	if !ok {
		return emptyContent(), apperr.New(apperr.NotFound, "返信・転送の元メッセージが見つかりません。").ForAccount(ref.AccountID)
	}
	src := sourceMessage(msg)

	cs.setSender(acct)
	cs.thread = &dto.ThreadRef{AccountID: ref.AccountID, ThreadID: ref.ThreadID}
	cs.threadAccountID = ref.AccountID

	if cs.mode != model.ModeForward {
		// 転送は新しいスレッドとして送る（Gmail は件名が変わる転送を元スレッドに入れない）。
		cs.sendThreadID = ref.ThreadID
		cs.inReplyTo = foldLine(src.MessageID)
		cs.references = referencesFor(src)
	} else {
		for _, a := range src.Attachments {
			cs.attachments = append(cs.attachments, sessionAttachment{
				ID: store.NewID(), Filename: a.Filename, MimeType: a.MimeType, Size: a.Size,
				Source: attachmentForwarded, MessageID: src.ID, GmailAtt: a.ID,
			})
		}
	}

	rcpt := reply.Resolve(cs.mode, src, strings.ToLower(acct.Email))
	quoteHTML, _ := quote.Build(cs.mode, src)
	content := emptyContent()
	content.To, content.Cc = cleanAddrs(rcpt.To), cleanAddrs(rcpt.Cc)
	content.Subject = reply.Subject(cs.mode, src.Subject)
	content.BodyHTML = signature.Insert(quoteHTML, acct.ID, acct.SignatureHTML)
	return content, nil
}

// restoreDraft は Gmail 下書きの内容でセッションの入力内容と添付を作る（F-12）。
// 添付は下書きに保存されたものが正で、返信・転送の元から作った添付（転送元添付）は置き換える
// （下書きで削除した転送元添付が復活しないように）。下書きの更新でメッセージ ID が変わるため、
// 添付はこのとき取得してセッションのメモリに保持する。合計が上限を超えるとき（Gmail 上で作られた下書き等）は
// 添付を復元せず、理由を attErr で返す（本文・宛先は復元する）。取得に失敗したときは err を返す
// （添付を黙って落として再保存で失うことがないように、下書きを開かない）。
func (s *Service) restoreDraft(ctx context.Context, cs *composeSession, draftID string) (content dto.ComposeContent, attErr, err error) {
	d, err := s.d.Gmail.GetDraft(ctx, cs.accountID, draftID)
	if err != nil {
		return emptyContent(), nil, apperr.FromInfra(err, cs.accountID)
	}
	var declared int64
	for _, a := range d.Attachments {
		declared += a.Size
	}
	var restored []sessionAttachment
	if declared > attachmentLimitBytes {
		attErr = errAttachmentTooLarge()
	} else if restored, err = s.fetchDraftAttachments(ctx, cs.accountID, d); err != nil {
		return emptyContent(), nil, err
	}
	cs.attachments = restored
	cs.draftID, cs.draftAcctID = draftID, cs.accountID
	body := d.Message.BodyHTML
	if body == "" {
		body = bodyhtml.TextToHTML(d.Message.BodyText)
	}
	content = emptyContent()
	content.To, content.Cc, content.Bcc = cleanAddrs(d.Message.To), cleanAddrs(d.Message.Cc), cleanAddrs(d.Bcc)
	content.Subject = foldLine(d.Message.Subject)
	content.BodyHTML = body
	return content, attErr, nil
}

// fetchDraftAttachments は下書きの添付の内容を取得して、セッションの添付にする。
func (s *Service) fetchDraftAttachments(ctx context.Context, accountID string, d gmail.DraftFull) ([]sessionAttachment, error) {
	out := make([]sessionAttachment, 0, len(d.Attachments))
	var total int64
	for _, a := range d.Attachments {
		data, err := s.d.Gmail.GetAttachment(ctx, accountID, d.Message.ID, a.ID)
		if err != nil {
			return nil, apperr.FromInfra(err, accountID)
		}
		if total += int64(len(data)); total > attachmentLimitBytes {
			return nil, errAttachmentTooLarge()
		}
		name := foldLine(a.Filename)
		if name == "" {
			name = "attachment"
		}
		mt := a.MimeType
		if mt == "" {
			mt = "application/octet-stream"
		}
		out = append(out, sessionAttachment{
			ID: store.NewID(), Filename: name, MimeType: mt, Size: int64(len(data)), Source: attachmentDraft, Data: data,
		})
	}
	return out, nil
}

// pickSource は返信・転送の元メッセージを選ぶ。下書きは除外する。wantID 省略時は、自分の送信
// （From が自アカウントのもの）を除いた最新。全部自分の送信なら最後のメッセージ。
func pickSource(msgs []gmail.MessageFull, selfEmail, wantID string) (gmail.MessageFull, bool) {
	live := make([]gmail.MessageFull, 0, len(msgs))
	for _, m := range msgs {
		if !slices.Contains(m.LabelIDs, labelDraft) {
			live = append(live, m)
		}
	}
	if len(live) == 0 {
		return gmail.MessageFull{}, false
	}
	if wantID != "" {
		for _, m := range live {
			if m.ID == wantID {
				return m, true
			}
		}
		return gmail.MessageFull{}, false
	}
	for i := len(live) - 1; i >= 0; i-- {
		if !strings.EqualFold(strings.TrimSpace(live[i].From.Email), selfEmail) {
			return live[i], true
		}
	}
	return live[len(live)-1], true
}

// sourceMessage は元メッセージを返信・転送の入力にする。ヘッダ由来の件名・表示名は CR/LF を畳む。
func sourceMessage(m gmail.MessageFull) model.SourceMessage {
	from := cleanAddrs([]model.Address{m.From})[0]
	return model.SourceMessage{
		ID: m.ID, ThreadID: m.ThreadID, MessageID: m.MessageID, References: m.References,
		From: from, ReplyTo: cleanAddrs(m.ReplyTo), To: cleanAddrs(m.To), Cc: cleanAddrs(m.Cc),
		Subject: foldLine(m.Subject), Date: m.InternalDate,
		BodyHTML: m.BodyHTML, BodyText: m.BodyText, Attachments: m.Attachments,
	}
}

// referencesFor は返信の References（元の References＋元の Message-ID。重複なし）。
func referencesFor(src model.SourceMessage) []string {
	refs := make([]string, 0, len(src.References)+1)
	for _, r := range src.References {
		if r = foldLine(r); r != "" {
			refs = append(refs, r)
		}
	}
	if id := foldLine(src.MessageID); id != "" && !slices.Contains(refs, id) {
		refs = append(refs, id)
	}
	return refs
}

// foldLine は CR/LF（連続は 1 つ）を空白 1 つに畳んで前後の空白を除く。メッセージのヘッダ由来の値
// （件名・表示名）が、作成画面の初期値や MIME ヘッダで改行を持ち込まないようにする。
func foldLine(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	return strings.TrimSpace(crlfRun.ReplaceAllString(s, " "))
}

var crlfRun = regexp.MustCompile(`[\r\n]+`)

// cleanAddrs は nil を空スライスにし、キャッシュ上のスライスを共有しないよう複製して、名前・アドレスの CR/LF を畳む。
func cleanAddrs(a []model.Address) []model.Address {
	out := make([]model.Address, 0, len(a))
	for _, x := range a {
		out = append(out, model.Address{Name: foldLine(x.Name), Email: foldLine(x.Email)})
	}
	return out
}
