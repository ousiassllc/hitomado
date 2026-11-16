// Package mcpcore は内蔵 MCP サーバー（mcpserver）の Backend 実装。読み取りと返信下書きの作成だけを公開する。
package mcpcore

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/bodyhtml"
	"github.com/ousiassllc/hitomado/internal/app/composecore"
	"github.com/ousiassllc/hitomado/internal/app/deps"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/mcpserver"
	"github.com/ousiassllc/hitomado/internal/store"
)

// maxBodyRunes は MCP に返す 1 メッセージの本文の上限（巨大なメールでクライアントの文脈を埋めない）。
const maxBodyRunes = 100_000

// InboxPages は MCP の一覧・検索が使う受信箱の読み取り（app.InboxService が満たす）。
type InboxPages interface {
	ListInbox(ctx context.Context, q dto.InboxQuery) (dto.ThreadPage, error)
	Search(ctx context.Context, q dto.SearchQuery) (dto.ThreadPage, error)
}

// Backend は mcpserver.Backend の実装。読み取りは既存のサービスと同じ経路（ローカル DB・スレッドの LRU・Gmail）で、
// 下書きは作成セッション（composecore）の OpenCompose→SaveDraft→CloseCompose だけを使う。
// 送信（PrepareSend／Send）は呼ばない。mcpserver.Backend に送信のメソッドが無いので、MCP からは呼べない。
type Backend struct {
	d       *deps.Deps
	inbox   InboxPages
	compose *composecore.Service
}

// New は Backend を作る。
func New(d *deps.Deps, inbox InboxPages) *Backend {
	return &Backend{d: d, inbox: inbox, compose: composecore.New(d)}
}

func (b *Backend) Status(ctx context.Context) (mcpserver.StatusInfo, error) {
	list, err := b.accounts(ctx)
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

// accounts は全アカウントを一覧の順で返す。
func (b *Backend) accounts(ctx context.Context) ([]store.Account, error) {
	list, err := b.d.Store.ListAccounts(ctx)
	if err != nil {
		return nil, apperr.FromInfra(err, "")
	}
	return list, nil
}

func accountInfo(id, email, name, status, statusMessage string) mcpserver.AccountInfo {
	return mcpserver.AccountInfo{ID: id, Email: email, DisplayName: name, Status: status, StatusMessage: statusMessage}
}

func (b *Backend) ListAccounts(ctx context.Context) ([]mcpserver.AccountInfo, error) {
	list, err := b.accounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]mcpserver.AccountInfo, len(list))
	for i, a := range list {
		out[i] = accountInfo(a.ID, a.Email, a.DisplayName, a.Status, a.StatusMessage)
	}
	return out, nil
}

func (b *Backend) ListThreads(ctx context.Context, q dto.InboxQuery) (dto.ThreadPage, error) {
	return b.inbox.ListInbox(ctx, q)
}

func (b *Backend) SearchThreads(ctx context.Context, q dto.SearchQuery) (dto.ThreadPage, error) {
	return b.inbox.Search(ctx, q)
}

// GetThread はスレッドを本文（プレーンテキスト）つきで返す。下書きは除き、時系列昇順。既読にはしない。
func (b *Backend) GetThread(ctx context.Context, accountID, threadID string) (mcpserver.ThreadView, error) {
	if _, err := b.d.Store.GetAccount(ctx, accountID); err != nil {
		return mcpserver.ThreadView{}, apperr.FromInfra(err, accountID)
	}
	tf, err := b.d.Threads.Get(ctx, accountID, threadID)
	if err != nil {
		return mcpserver.ThreadView{}, b.d.GmailErr(ctx, accountID, err)
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
	if view.DraftID, err = b.d.FindThreadDraft(ctx, accountID, threadID, tf.Messages); err != nil {
		return mcpserver.ThreadView{}, err
	}
	return view, nil
}

func mcpMessage(m gmail.MessageFull) mcpserver.ThreadMessageView {
	text := m.BodyText
	if text == "" && m.BodyHTML != "" {
		text = textconv.HTMLToText(m.BodyHTML)
	}
	v := mcpserver.ThreadMessageView{
		ID: m.ID, From: m.From, To: appcore.NonNilAddrs(m.To), Cc: appcore.NonNilAddrs(m.Cc), Date: m.InternalDate.Format(time.RFC3339),
		IsUnread: slices.Contains(m.LabelIDs, "UNREAD"), BodyText: text, Attachments: m.Attachments,
	}
	if v.Attachments == nil {
		v.Attachments = []model.AttachmentInfo{}
	}
	if utf8.RuneCountInString(text) > maxBodyRunes {
		v.BodyText, v.Truncated = string([]rune(text)[:maxBodyRunes]), true
	}
	return v
}

// CreateReplyDraft は返信の作成セッションを開き、本文の先頭に req.BodyText を置いて Gmail の下書きとして保存する。
// 送信元は OpenCompose が返信元スレッドのアカウントに固定する（SaveDraft も accounts.email から From を作る）。
// 宛先・件名・署名・引用は、アプリで返信ボタンを押したときと同じ規則で決まる。
func (b *Backend) CreateReplyDraft(ctx context.Context, req mcpserver.ReplyDraftRequest) (mcpserver.ReplyDraft, error) {
	if strings.TrimSpace(req.BodyText) == "" {
		return mcpserver.ReplyDraft{}, apperr.New(apperr.Validation, "返信の本文を指定してください。")
	}
	// 同じスレッドに下書きがすでにあるときは作らない（エージェントの再試行で下書きが増え続けないように）。
	// 既存の下書きの ID を返すので、利用者がアプリで開いて編集する。
	if existing, ok, err := b.d.Gmail.FindDraftForThread(ctx, req.AccountID, req.ThreadID); err != nil {
		return mcpserver.ReplyDraft{}, b.d.GmailErr(ctx, req.AccountID, err)
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
		To: appcore.NonNilAddrs(content.To), Cc: appcore.NonNilAddrs(content.Cc), Subject: content.Subject,
	}
	if sess.From != nil {
		out.From = sess.From.Email
	}
	return out, nil
}

var _ mcpserver.Backend = (*Backend)(nil)
