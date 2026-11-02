package appcore

import (
	"slices"

	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// labelDraft は下書きメッセージのラベル。
const labelDraft = "DRAFT"

// SummaryFromMeta は threads.get(metadata) から一覧行を組み立てる。labels はユーザー作成ラベル（store 由来）。
// DRAFT ラベルのメッセージは、最新メッセージ・件数・スニペット・差出人・件名・未読・添付などの集計から除く
// （下書きを保存するとスレッドが先頭に上がり差出人が自分になるため）。下書きしか無いスレッドは ok=false。
func SummaryFromMeta(accountID string, meta gmail.ThreadMeta, labels []store.Label) (sum dto.ThreadSummary, ok bool) {
	msgs := make([]gmail.MessageMeta, 0, len(meta.Messages))
	for _, m := range meta.Messages {
		if !slices.Contains(m.LabelIDs, labelDraft) {
			msgs = append(msgs, m)
		}
	}
	sum = dto.ThreadSummary{AccountID: accountID, ThreadID: meta.ID, Labels: []dto.Label{}, MessageCount: len(msgs)}
	if len(msgs) == 0 {
		return sum, false
	}
	present := map[string]struct{}{}
	var last gmail.MessageMeta
	for i, m := range msgs {
		if i == 0 {
			sum.Subject = m.Subject
		}
		if i == 0 || !m.InternalDate.Before(last.InternalDate) {
			last = m
		}
		sum.HasAttachment = sum.HasAttachment || m.HasAttachment
		for _, l := range m.LabelIDs {
			switch l {
			case "UNREAD":
				sum.IsUnread = true
			case "INBOX":
				sum.InInbox = true
			}
			present[l] = struct{}{}
		}
	}
	sum.Snippet = last.Snippet
	sum.LastFrom = last.From
	sum.LastMessageAt = last.InternalDate.UnixMilli()
	for _, l := range labels {
		if _, ok := present[l.ID]; ok {
			sum.Labels = append(sum.Labels, dto.Label{ID: l.ID, Name: l.Name, ColorBg: l.ColorBg, ColorFg: l.ColorFg})
		}
	}
	return sum, true
}
