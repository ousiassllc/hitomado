package appcore

import (
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// SummaryFromMeta は threads.get(metadata) から一覧行を組み立てる。labels はユーザー作成ラベル（store 由来）。
func SummaryFromMeta(accountID string, meta gmail.ThreadMeta, labels []store.Label) dto.ThreadSummary {
	sum := dto.ThreadSummary{AccountID: accountID, ThreadID: meta.ID, Labels: []dto.Label{}, MessageCount: len(meta.Messages)}
	if len(meta.Messages) == 0 {
		return sum
	}
	present := map[string]struct{}{}
	var last gmail.MessageMeta
	for i, m := range meta.Messages {
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
	return sum
}
