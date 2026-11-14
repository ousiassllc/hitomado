package gmail

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"sync"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/ousiassllc/hitomado/internal/gmail/mimeparse"
)

// draftFetchWorkers は ListDrafts が 1 アカウントの下書きを同時に取得する数。
// 呼び出しはアカウントのレート制限（units）を通るので、ここは待ち時間を重ねるだけの小さな値でよい。
const draftFetchWorkers = 4

// maxDraftsPage は drafts.list の 1 ページの最大件数（Gmail の上限）。
const maxDraftsPage = 500

// ListDrafts は下書きを先頭から limit 件まで取得する（drafts.list の ID を集めてから、drafts.get format=metadata で件名などを読む）。
func (c *client) ListDrafts(ctx context.Context, accountID string, limit int) (DraftList, error) {
	if limit < 1 {
		return DraftList{Items: []DraftSummary{}}, nil
	}
	ids, truncated, err := c.listDraftIDs(ctx, accountID, limit)
	if err != nil {
		return DraftList{}, err
	}
	found := make([]*DraftSummary, len(ids))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		sem      = make(chan struct{}, draftFetchWorkers)
	)
	for i, id := range ids {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s, err := c.getDraftSummary(ctx, accountID, id)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case errors.Is(err, ErrNotFound): // 一覧のあとで削除された下書き
			case err != nil:
				if firstErr == nil {
					firstErr = err
					cancel()
				}
			default:
				found[i] = &s
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return DraftList{}, firstErr
	}
	out := DraftList{Items: make([]DraftSummary, 0, len(found)), Truncated: truncated}
	for _, s := range found {
		if s != nil {
			out.Items = append(out.Items, *s)
		}
	}
	return out, nil
}

// listDraftIDs は drafts.list を、limit+1 件（超過の判定用）に届くか、末尾まで辿って、下書きの ID を返す。
func (c *client) listDraftIDs(ctx context.Context, accountID string, limit int) (ids []string, truncated bool, err error) {
	pageToken := ""
	for len(ids) <= limit {
		var resp *gmailapi.ListDraftsResponse
		err := c.do(ctx, accountID, callSpec{name: "drafts.list", units: unitsDraft, retry5xx: true}, func(s *gmailapi.Service) error {
			call := s.Users.Drafts.List(userID).MaxResults(int64(min(limit+1-len(ids), maxDraftsPage))).Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			var err error
			resp, err = call.Do()
			return err
		})
		if err != nil {
			return nil, false, err
		}
		for _, d := range resp.Drafts {
			ids = append(ids, d.Id)
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	if len(ids) > limit {
		return ids[:limit], true, nil
	}
	return ids, false, nil
}

// getDraftSummary は drafts.get format=metadata で 1 件の下書きの一覧用の項目を読む。
func (c *client) getDraftSummary(ctx context.Context, accountID, draftID string) (DraftSummary, error) {
	var d *gmailapi.Draft
	err := c.do(ctx, accountID, callSpec{name: "drafts.get", units: unitsDraft, retry5xx: true}, func(s *gmailapi.Service) error {
		var err error
		d, err = s.Users.Drafts.Get(userID, draftID).Format("metadata").Context(ctx).Do()
		return err
	})
	if err != nil {
		return DraftSummary{}, err
	}
	if d.Message == nil {
		return DraftSummary{}, fmt.Errorf("gmail: drafts.get: 下書きにメッセージがありません: %w", ErrNotFound)
	}
	meta := metaFromMessage(d.Message)
	s := DraftSummary{
		ID: d.Id, ThreadID: meta.ThreadID, UpdatedAt: meta.InternalDate,
		To: meta.To, Subject: meta.Subject, Snippet: strings.TrimSpace(html.UnescapeString(meta.Snippet)),
	}
	if d.Message.Payload != nil {
		s.IsReply = strings.TrimSpace(mimeparse.HeaderValue(d.Message.Payload.Headers, "In-Reply-To")) != ""
	}
	return s, nil
}
