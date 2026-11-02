package app

import (
	"context"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/store"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 100
	// maxFetchRounds は 1 回の ListInbox が Gmail から追加取得する最大回数（未読のみ等で一覧が埋まらない場合の歯止め）。
	maxFetchRounds = 5
)

// InboxService は統合受信箱・検索・未読件数・スレッド操作・ラベル（バインディング仕様 §3）。
// 検索は inbox_search.go、スレッド操作は inbox_ops.go、ラベルは inbox_labels.go。
type InboxService struct{ d *Deps }

// NewInboxService は InboxService を作る。
func NewInboxService(d *Deps) *InboxService { return &InboxService{d: d} }

func pageLimit(n int) (int, error) {
	switch {
	case n == 0:
		return defaultPageLimit, nil
	case n < 0 || n > maxPageLimit:
		return 0, validation("limit は 1〜100 で指定してください。")
	}
	return n, nil
}

// ListInbox はローカル DB の受信箱をキーセットページングで返す。ローカルの行が尽き、かつ未取得分のある
// アカウントがあれば Gmail から追加取得して続きを返す。アカウント別の失敗は Failures に入れ、他は返す。
func (s *InboxService) ListInbox(ctx context.Context, q dto.InboxQuery) (dto.ThreadPage, error) {
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return dto.ThreadPage{}, err
	}
	after, err := appcore.DecodeInboxCursor(q.Cursor)
	if err != nil {
		return dto.ThreadPage{}, err
	}
	list, _, err := accountsByID(s.d, ctx)
	if err != nil {
		return dto.ThreadPage{}, err
	}
	targets := targetAccounts(list, q.AccountIDs)
	page := dto.ThreadPage{Items: []dto.ThreadSummary{}, Failures: []dto.AccountFailure{}}
	if len(targets) == 0 {
		return page, nil
	}
	ids := make([]string, len(targets))
	for i, a := range targets {
		ids[i] = a.ID
	}
	filter := store.InboxFilter{AccountIDs: ids, UnreadOnly: q.UnreadOnly, After: after, Limit: limit + 1}

	failed := map[string]bool{}
	var rows []store.ThreadRow
	canFetch := true // Gmail からの追加取得でまだ進める見込みがあるか
	for round := 0; ; round++ {
		if rows, err = s.d.Store.ListInbox(ctx, filter); err != nil {
			return dto.ThreadPage{}, apperr.FromInfra(err, "")
		}
		if len(rows) > limit || round >= maxFetchRounds {
			break
		}
		if !s.fetchMore(ctx, ids, failed, &page) {
			canFetch = false
			break
		}
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	for _, r := range rows {
		page.Items = append(page.Items, summaryFromRow(r))
	}
	// 続きがある: ローカルに余りがある、または Gmail 側に未取得の受信箱が残っている。
	// 条件に合う行が 0 件でも後者なら、同じ位置（カーソルなしなら先頭）を指すカーソルを返して続きを取れるようにする。
	if more || (canFetch && s.hasRemote(ctx, ids, failed)) {
		next := after
		if len(rows) > 0 {
			last := rows[len(rows)-1]
			next = &store.InboxCursor{LastMessageAt: last.LastMessageAt, AccountID: last.AccountID, ThreadID: last.ThreadID}
		}
		page.NextCursor = appcore.EncodeInboxCursor(next)
	}
	return page, nil
}

// fetchMore は未取得分のあるアカウントすべてから 1 ページ追加取得する。
// 1 件でも進んだ（新規スレッドを保存した、または取得位置のトークンが進んだ・末尾に達した）なら true。
func (s *InboxService) fetchMore(ctx context.Context, ids []string, failed map[string]bool, page *dto.ThreadPage) bool {
	progressed := false
	for _, id := range ids {
		if failed[id] {
			continue
		}
		before, err := s.d.Store.GetSyncState(ctx, id)
		if err == nil && before.InboxExhausted {
			continue
		}
		added := 0
		if err == nil {
			added, err = s.d.Sync.FetchMore(ctx, id)
		}
		var after store.SyncState
		if err == nil {
			after, err = s.d.Store.GetSyncState(ctx, id)
		}
		if err != nil {
			failed[id] = true
			page.Failures = append(page.Failures, dto.AccountFailure{AccountID: id, Error: apperr.ToMap(gmailErr(s.d, ctx, id, err))})
			continue
		}
		progressed = progressed || added > 0 || after.InboxExhausted || after.InboxPageToken != before.InboxPageToken
	}
	return progressed
}

// hasRemote は Gmail 側に未取得の受信箱が残っているアカウントがあるかを返す（失敗したアカウントは除く）。
func (s *InboxService) hasRemote(ctx context.Context, ids []string, failed map[string]bool) bool {
	for _, id := range ids {
		if failed[id] {
			continue
		}
		if state, err := s.d.Store.GetSyncState(ctx, id); err == nil && !state.InboxExhausted {
			return true
		}
	}
	return false
}

// Refresh は差分同期を今すぐ実行して完了を待つ。空は全アカウント。
func (s *InboxService) Refresh(ctx context.Context, accountIDs []string) (dto.RefreshResponse, error) {
	results := s.d.Sync.Refresh(ctx, accountIDs)
	out := dto.RefreshResponse{Results: make([]dto.RefreshResult, len(results))}
	for i, r := range results {
		out.Results[i] = dto.RefreshResult{AccountID: r.AccountID, OK: r.Err == nil}
		if r.Err != nil {
			out.Results[i].Error = apperr.ToMap(gmailErr(s.d, ctx, r.AccountID, r.Err))
		}
	}
	return out, nil
}

// GetUnreadCounts はアカウントごとの受信箱の未読スレッド数を返す。
func (s *InboxService) GetUnreadCounts(ctx context.Context) (map[string]int, error) {
	m, err := s.d.Store.UnreadCounts(ctx)
	if err != nil {
		return nil, apperr.FromInfra(err, "")
	}
	if m == nil {
		m = map[string]int{}
	}
	return m, nil
}
