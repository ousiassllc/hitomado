package app

import (
	"context"
	"slices"
	"sort"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/domain/model"
	"github.com/ousiassllc/hitomado/internal/gmail"
)

// ThreadService はスレッド詳細・添付保存・リンクを開く（バインディング仕様 §4）。
// 添付保存・リンクは thread_actions.go、本文 HTML の加工は thread_html.go。
type ThreadService struct{ d *Deps }

// NewThreadService は ThreadService を作る。
func NewThreadService(d *Deps) *ThreadService { return &ThreadService{d: d} }

// GetThread はスレッド詳細を返す。下書きは除き、時系列昇順。状態は変更しない（既読化は MarkRead）。
func (s *ThreadService) GetThread(ctx context.Context, ref dto.ThreadRef) (dto.ThreadDetail, error) {
	acc, err := s.d.Store.GetAccount(ctx, ref.AccountID)
	if err != nil {
		return dto.ThreadDetail{}, apperr.FromInfra(err, ref.AccountID)
	}
	tf, err := s.d.Threads.Get(ctx, ref.AccountID, ref.ThreadID)
	if err != nil {
		return dto.ThreadDetail{}, gmailErr(s.d, ctx, ref.AccountID, err)
	}
	labels, err := s.d.Store.ListLabels(ctx, ref.AccountID)
	if err != nil {
		return dto.ThreadDetail{}, apperr.FromInfra(err, ref.AccountID)
	}

	// threadcache の値はキャッシュと共有されるため、並べ替える前に複製する。
	msgs := slices.DeleteFunc(slices.Clone(tf.Messages), func(m gmail.MessageFull) bool { return slices.Contains(m.LabelIDs, "DRAFT") })
	sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].InternalDate.Before(msgs[j].InternalDate) })

	detail := dto.ThreadDetail{
		Account: accountDTO(acc), ThreadID: ref.ThreadID, Messages: make([]dto.ThreadMessage, len(msgs)), Labels: []dto.Label{},
	}
	if len(msgs) > 0 {
		detail.Subject = msgs[0].Subject
	} else if len(tf.Messages) > 0 {
		detail.Subject = tf.Messages[0].Subject
	}
	present := map[string]struct{}{}
	for i, m := range msgs {
		detail.Messages[i] = threadMessage(m)
		for _, l := range m.LabelIDs {
			present[l] = struct{}{}
		}
	}
	for _, l := range userLabelsDTO(labels) {
		if _, ok := present[l.ID]; ok {
			detail.Labels = append(detail.Labels, l)
		}
	}
	draftID, ok, err := s.d.Gmail.FindDraftForThread(ctx, ref.AccountID, ref.ThreadID)
	switch {
	case err != nil && ctx.Err() != nil:
		return dto.ThreadDetail{}, apperr.FromInfra(ctx.Err(), ref.AccountID)
	case err != nil:
		// 下書きの有無が分からなくてもスレッドは読めるようにする。
		logger(s.d).Warn("スレッドの下書き検索に失敗", "accountId", ref.AccountID, "err", err)
	case ok:
		detail.DraftID = draftID
	}
	return detail, nil
}

func threadMessage(m gmail.MessageFull) dto.ThreadMessage {
	tm := dto.ThreadMessage{
		ID: m.ID, From: m.From, To: nonNilAddrs(m.To), Cc: nonNilAddrs(m.Cc), Date: m.InternalDate.UnixMilli(),
		IsUnread: slices.Contains(m.LabelIDs, "UNREAD"), Attachments: m.Attachments,
	}
	if tm.Attachments == nil {
		tm.Attachments = []model.AttachmentInfo{}
	}
	if m.BodyHTML != "" {
		tm.BodyHTML = appcore.InlineCIDImages(m.BodyHTML, m.InlineImages)
		tm.HasRemoteImages = appcore.HasRemoteImages(m.BodyHTML)
	} else {
		tm.BodyText = m.BodyText
	}
	return tm
}
