package app

import (
	"context"
	"slices"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/bodyhtml"
	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/mcpserver"
)

// maxMCPBodyRunes は MCP に返す 1 メッセージの本文の上限（巨大なメールでクライアントの文脈を埋めない）。
const maxMCPBodyRunes = 100_000

// mcpBackend は mcpserver.Backend の実装。読み取りは既存のサービスと同じ経路（ローカル DB・スレッドの LRU・Gmail）で、
// 下書きは作成セッション（composecore）の OpenCompose→SaveDraft→CloseCompose だけを使う。
// 送信（PrepareSend／Send）は呼ばない。mcpserver.Backend に送信のメソッドが無いので、MCP からは呼べない。
type mcpBackend struct {
	d       *Deps
	inbox   *InboxService
	compose *composecore.Service
}

func newMCPBackend(d *Deps, inbox *InboxService) *mcpBackend {
	return &mcpBackend{d: d, inbox: inbox, compose: composecore.New((*deps.Deps)(d))}
}

func (b *mcpBackend) Status(ctx context.Context) (mcpserver.StatusInfo, error) {
	list, _, err := accountsByID(b.d, ctx)
	if err != nil {
		return mcpserver.StatusInfo{}, err
	}
	out := mcpserver.StatusInfo{Version: b.d.Version, Accounts: make([]mcpserver.AccountSync, len(list))}
	for i, a := range list {
		as := mcpserver.AccountSync{AccountInfo: accountInfo(a.ID, a.Email, a.DisplayName, a.Status, a.StatusMessage)}
		// 同期状態が無い（まだ一度も同期していない）アカウントは既定値のまま。
		if st, err := b.d.Store.GetSyncState(ctx, a.ID); err == nil {
			as.InitialSyncDone, as.LastError = st.InitialSyncDone, st.LastError
			if !st.LastSyncedAt.IsZero() {
				as.LastSyncedAt = st.LastSyncedAt.Format(time.RFC3339)
			}
		}
		out.Accounts[i] = as
	}
	return out, nil
}

func accountInfo(id, email, name, status, statusMessage string) mcpserver.AccountInfo {
	return mcpserver.AccountInfo{ID: id, Email: email, DisplayName: name, Status: status, StatusMessage: statusMessage}
}

func (b *mcpBackend) ListAccounts(ctx context.Context) ([]mcpserver.AccountInfo, error) {
	list, _, err := accountsByID(b.d, ctx)
	if err != nil {
		return nil, err
	}
	out := make([]mcpserver.AccountInfo, len(list))
	for i, a := range list {
		out[i] = accountInfo(a.ID, a.Email, a.DisplayName, a.Status, a.StatusMessage)
	}
	return out, nil
}

func (b *mcpBackend) ListThreads(ctx context.Context, q dto.InboxQuery) (dto.ThreadPage, error) {
	return b.inbox.ListInbox(ctx, q)
}

func (b *mcpBackend) SearchThreads(ctx context.Context, q dto.SearchQuery) (dto.ThreadPage, error) {
	return b.inbox.Search(ctx, q)
}

// GetThread はスレッドを本文（プレーンテキスト）つきで返す。下書きは除き、時系列昇順。既読にはしない。
func (b *mcpBackend) GetThread(ctx context.Context, accountID, threadID string) (mcpserver.ThreadView, error) {
	if _, err := b.d.Store.GetAccount(ctx, accountID); err != nil {
		return mcpserver.ThreadView{}, apperr.FromInfra(err, accountID)
	}
	tf, err := b.d.Threads.Get(ctx, accountID, threadID)
	if err != nil {
		return mcpserver.ThreadView{}, gmailErr(b.d, ctx, accountID, err)
	}
	// threadcache の値はキャッシュと共有されるため、並べ替える前に複製する。
	msgs := slices.DeleteFunc(slices.Clone(tf.Messages), func(m gmail.MessageFull) bool { return slices.Contains(m.LabelIDs, "DRAFT") })
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].InternalDate.Before(msgs[j].InternalDate) })

	view := mcpserver.ThreadView{AccountID: accountID, ThreadID: threadID, Messages: make([]mcpserver.ThreadMessageView, len(msgs))}
	if len(msgs) > 0 {
		view.Subject = msgs[0].Subject
	}
	for i, m := range msgs {
		view.Messages[i] = mcpMessage(m)
	}
	draftID, ok, err := b.d.Gmail.FindDraftForThread(ctx, accountID, threadID)
	switch {
	case err != nil && ctx.Err() != nil:
		return mcpserver.ThreadView{}, apperr.FromInfra(ctx.Err(), accountID)
	case err != nil:
		logger(b.d).Warn("スレッドの下書き検索に失敗", "accountId", accountID, "err", err)
	case ok:
		view.DraftID = draftID
	}
	return view, nil
}

func mcpMessage(m gmail.MessageFull) mcpserver.ThreadMessageView {
	text := m.BodyText
	if text == "" && m.BodyHTML != "" {
		text = textconv.HTMLToText(m.BodyHTML)
	}
	v := mcpserver.ThreadMessageView{
		ID: m.ID, From: m.From, To: nonNilAddrs(m.To), Cc: nonNilAddrs(m.Cc), Date: m.InternalDate.Format(time.RFC3339),
		IsUnread: slices.Contains(m.LabelIDs, "UNREAD"), BodyText: text, Attachments: m.Attachments,
	}
	if v.Attachments == nil {
		v.Attachments = []model.AttachmentInfo{}
	}
	if utf8.RuneCountInString(text) > maxMCPBodyRunes {
		v.BodyText, v.Truncated = string([]rune(text)[:maxMCPBodyRunes]), true
	}
	return v
}

// CreateReplyDraft は返信の作成セッションを開き、本文の先頭に req.BodyText を置いて Gmail の下書きとして保存する。
// 送信元は OpenCompose が返信元スレッドのアカウントに固定する（SaveDraft も accounts.email から From を作る）。
// 宛先・件名・署名・引用は、アプリで返信ボタンを押したときと同じ規則で決まる。
func (b *mcpBackend) CreateReplyDraft(ctx context.Context, req mcpserver.ReplyDraftRequest) (mcpserver.ReplyDraft, error) {
	if trimmed(req.BodyText) == "" {
		return mcpserver.ReplyDraft{}, validation("返信の本文を指定してください。")
	}
	// 同じスレッドに下書きがすでにあるときは作らない（エージェントの再試行で下書きが増え続けないように）。
	// 既存の下書きの ID を返すので、利用者がアプリで開いて編集する。
	if existing, ok, err := b.d.Gmail.FindDraftForThread(ctx, req.AccountID, req.ThreadID); err != nil {
		return mcpserver.ReplyDraft{}, gmailErr(b.d, ctx, req.AccountID, err)
	} else if ok {
		return mcpserver.ReplyDraft{}, apperr.New(apperr.Validation,
			"このスレッドにはすでに下書き（"+existing+"）があります。アプリで開いて編集してください。").ForAccount(req.AccountID)
	}
	mode := model.ModeReply
	if req.ReplyAll {
		mode = model.ModeReplyAll
	}
	sess, err := b.compose.OpenCompose(ctx, dto.OpenComposeRequest{
		Mode: string(mode), Ref: &dto.ThreadRef{AccountID: req.AccountID, ThreadID: req.ThreadID}, SourceMessageID: req.SourceMessageID,
	})
	if err != nil {
		return mcpserver.ReplyDraft{}, err
	}
	// 下書きは Gmail に残す。セッションだけ閉じる。
	defer func() { _ = b.compose.CloseCompose(context.WithoutCancel(ctx), sess.SessionID) }()

	content := sess.Content
	content.BodyHTML = bodyhtml.TextToHTML(req.BodyText) + content.BodyHTML // 署名・引用の前に返信本文を置く
	saved, err := b.compose.SaveDraft(ctx, sess.SessionID, content)
	if err != nil {
		return mcpserver.ReplyDraft{}, err
	}
	out := mcpserver.ReplyDraft{
		DraftID: saved.DraftID, AccountID: sess.AccountID, ThreadID: req.ThreadID,
		To: nonNilAddrs(content.To), Cc: nonNilAddrs(content.Cc), Subject: content.Subject,
	}
	if sess.From != nil {
		out.From = sess.From.Email
	}
	return out, nil
}

var _ mcpserver.Backend = (*mcpBackend)(nil)
