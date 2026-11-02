package gmail

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

// metadataHeaders は一覧同期（format=metadata）で取得するヘッダ。
var metadataHeaders = []string{"From", "To", "Cc", "Subject", "Date"}

// Profile は users.getProfile を呼ぶ。
func (c *client) Profile(ctx context.Context, accountID string) (Profile, error) {
	var out Profile
	err := c.do(ctx, accountID, callSpec{name: "getProfile", units: unitsLabels, retry5xx: true}, func(s *gmailapi.Service) error {
		p, err := s.Users.GetProfile(userID).Context(ctx).Do()
		if err != nil {
			return err
		}
		out = Profile{Email: strings.ToLower(p.EmailAddress), HistoryID: historyString(p.HistoryId)}
		return nil
	})
	return out, err
}

// ListHistory は startHistoryID 以降の変更を全ページ取得して統合する。失効（404）は ErrHistoryExpired。
func (c *client) ListHistory(ctx context.Context, accountID, startHistoryID string) (HistoryPage, error) {
	start, err := strconv.ParseUint(startHistoryID, 10, 64)
	if err != nil {
		return HistoryPage{}, fmt.Errorf("gmail: ListHistory: startHistoryId が不正です: %w", ErrHistoryExpired)
	}
	out := HistoryPage{LatestHistoryID: startHistoryID, ChangedThreadIDs: []string{}, Added: []HistoryAdded{}}
	seen := map[string]bool{}
	pageToken := ""
	for {
		var resp *gmailapi.ListHistoryResponse
		err := c.do(ctx, accountID, callSpec{name: "history.list", units: unitsHistory, retry5xx: true}, func(s *gmailapi.Service) error {
			call := s.Users.History.List(userID).StartHistoryId(start).Context(ctx).
				HistoryTypes("messageAdded", "messageDeleted", "labelAdded", "labelRemoved")
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			var err error
			resp, err = call.Do()
			return err
		})
		if errors.Is(err, ErrNotFound) {
			return HistoryPage{}, fmt.Errorf("gmail: history.list: %w", ErrHistoryExpired)
		}
		if err != nil {
			return HistoryPage{}, err
		}
		if resp.HistoryId != 0 {
			out.LatestHistoryID = historyString(resp.HistoryId)
		}
		for _, h := range resp.History {
			collectHistory(h, &out, seen)
		}
		if resp.NextPageToken == "" {
			return out, nil
		}
		pageToken = resp.NextPageToken
	}
}

// collectHistory は history の 1 レコードから変更のあったスレッドと追加メッセージを集める。
func collectHistory(h *gmailapi.History, out *HistoryPage, seen map[string]bool) {
	touch := func(m *gmailapi.Message) {
		if m != nil && m.ThreadId != "" && !seen[m.ThreadId] {
			seen[m.ThreadId] = true
			out.ChangedThreadIDs = append(out.ChangedThreadIDs, m.ThreadId)
		}
	}
	for _, a := range h.MessagesAdded {
		touch(a.Message)
		if a.Message != nil {
			out.Added = append(out.Added, HistoryAdded{
				MessageID: a.Message.Id, ThreadID: a.Message.ThreadId, LabelIDs: append([]string{}, a.Message.LabelIds...),
			})
		}
	}
	for _, d := range h.MessagesDeleted {
		touch(d.Message)
	}
	for _, l := range h.LabelsAdded {
		touch(l.Message)
	}
	for _, l := range h.LabelsRemoved {
		touch(l.Message)
	}
}

// ListThreads は threads.list を呼ぶ。
func (c *client) ListThreads(ctx context.Context, accountID string, q ListQuery) (ThreadList, error) {
	out := ThreadList{Threads: []ThreadStub{}}
	err := c.do(ctx, accountID, callSpec{name: "threads.list", units: unitsThread, retry5xx: true}, func(s *gmailapi.Service) error {
		call := s.Users.Threads.List(userID).Context(ctx)
		if q.Q != "" {
			call = call.Q(q.Q)
		}
		if len(q.LabelIDs) > 0 {
			call = call.LabelIds(q.LabelIDs...)
		}
		if q.PageToken != "" {
			call = call.PageToken(q.PageToken)
		}
		if q.MaxResults > 0 {
			call = call.MaxResults(q.MaxResults)
		}
		resp, err := call.Do()
		if err != nil {
			return err
		}
		out.Threads = out.Threads[:0]
		for _, t := range resp.Threads {
			out.Threads = append(out.Threads, ThreadStub{ID: t.Id, HistoryID: historyString(t.HistoryId), Snippet: t.Snippet})
		}
		out.NextPageToken = resp.NextPageToken
		return nil
	})
	return out, err
}

// GetThreadMetadata は threads.get(format=metadata) を呼ぶ。無ければ ErrNotFound。
func (c *client) GetThreadMetadata(ctx context.Context, accountID, threadID string) (ThreadMeta, error) {
	var t *gmailapi.Thread
	err := c.do(ctx, accountID, callSpec{name: "threads.get", units: unitsThread, retry5xx: true}, func(s *gmailapi.Service) error {
		var err error
		t, err = s.Users.Threads.Get(userID, threadID).Format("metadata").MetadataHeaders(metadataHeaders...).Context(ctx).Do()
		return err
	})
	if err != nil {
		return ThreadMeta{}, err
	}
	out := ThreadMeta{ID: t.Id, HistoryID: historyString(t.HistoryId), Messages: make([]MessageMeta, 0, len(t.Messages))}
	for _, m := range t.Messages {
		out.Messages = append(out.Messages, metaFromMessage(m))
	}
	return out, nil
}

// GetThreadFull は threads.get(format=full) を呼び、本文・添付・インライン画像を取り出す。
func (c *client) GetThreadFull(ctx context.Context, accountID, threadID string) (ThreadFull, error) {
	var t *gmailapi.Thread
	err := c.do(ctx, accountID, callSpec{name: "threads.get", units: unitsThread, retry5xx: true}, func(s *gmailapi.Service) error {
		var err error
		t, err = s.Users.Threads.Get(userID, threadID).Format("full").Context(ctx).Do()
		return err
	})
	if err != nil {
		return ThreadFull{}, err
	}
	out := ThreadFull{ID: t.Id, HistoryID: historyString(t.HistoryId), Messages: make([]MessageFull, 0, len(t.Messages))}
	for _, m := range t.Messages {
		mf, err := c.messageFull(ctx, accountID, m)
		if err != nil {
			return ThreadFull{}, err
		}
		out.Messages = append(out.Messages, mf)
	}
	return out, nil
}

// historyString は uint64 の historyId を文字列にする（0＝未設定は空文字）。
func historyString(id uint64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}
