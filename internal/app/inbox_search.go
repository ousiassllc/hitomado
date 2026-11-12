package app

import (
	"context"
	"errors"
	"slices"
	"sort"
	"sync"

	"github.com/ousiassllc/hitomado/internal/app/appcore"
	"github.com/ousiassllc/hitomado/internal/app/apperr"
	"github.com/ousiassllc/hitomado/internal/app/dto"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/logging"
	"github.com/ousiassllc/hitomado/internal/store"
)

// searchEntry は検索結果の 1 件。Sum は取得済みのときのみ（カーソル由来の残りは配信時に取得する）。
type searchEntry struct {
	AccountID string
	ThreadID  string
	T         int64
	Sum       *dto.ThreadSummary
}

// accountSearch は 1 アカウントの検索の作業状態。
type accountSearch struct {
	acc    store.Account
	labels []store.Label // ユーザー作成ラベル
	queue  []searchEntry // 最終メッセージ日時の降順
	pt     string        // 次の pageToken（空なら末尾）
	err    error
	gone   bool // 再試行しても直らない失敗（続きのカーソルに残さない）
}

// Search は Gmail 検索クエリを各アカウントで並列実行し、最終メッセージ日時の降順で統合する（結果は保存しない）。
// カーソルはアカウントごとの pageToken と未配信の残りを埋め込んだ不透明文字列で、サーバー側に状態を持たない。
func (s *InboxService) Search(ctx context.Context, q dto.SearchQuery) (dto.ThreadPage, error) {
	query := trimmed(q.Q)
	if query == "" {
		return dto.ThreadPage{}, validation("検索クエリを入力してください。")
	}
	limit, err := pageLimit(q.Limit)
	if err != nil {
		return dto.ThreadPage{}, err
	}
	states, err := appcore.DecodeSearchCursor(q.Cursor)
	if err != nil {
		return dto.ThreadPage{}, err
	}
	list, _, err := accountsByID(s.d, ctx)
	if err != nil {
		return dto.ThreadPage{}, err
	}
	var work []*accountSearch
	for _, a := range targetAccounts(list, q.AccountIDs) {
		if states != nil {
			if _, ok := states[a.ID]; !ok {
				continue // 前のページまでに出し尽くしたアカウント
			}
		}
		work = append(work, &accountSearch{acc: a})
	}
	page := dto.ThreadPage{Items: []dto.ThreadSummary{}, Failures: []dto.AccountFailure{}}

	appcore.ParallelDo(logger(s.d), len(work), len(work), func(i int) { s.loadAccount(ctx, work[i], query, limit, states[work[i].acc.ID]) })
	next := map[string]appcore.SearchState{}
	var all []searchEntry
	for _, w := range work {
		if w.err != nil {
			page.Failures = append(page.Failures, s.failure(ctx, w.acc.ID, w.err))
			w.gone = apperr.IsPermanent(w.err)
			continue // 失敗したアカウントは配信せず、取得済みの分も含めて next に引き継ぐ（時刻順を崩さないため）。
		}
		all = append(all, w.queue...)
	}
	sort.SliceStable(all, func(i, j int) bool { return entryLess(all[i], all[j]) })
	picked := all[:min(limit, len(all))]

	s.hydratePicked(ctx, work, picked, &page)
	delivered := map[string]struct{}{}
	for _, e := range picked {
		if e.Sum != nil {
			page.Items = append(page.Items, *e.Sum)
			delivered[e.AccountID+"/"+e.ThreadID] = struct{}{}
		}
	}
	for _, w := range work {
		if w.gone {
			continue // 再接続待ち・資格情報拒否・アカウント消滅は再試行しても直らない。Failures にだけ載せる。
		}
		st := appcore.SearchState{PT: w.pt}
		for _, e := range w.queue {
			if _, ok := delivered[e.AccountID+"/"+e.ThreadID]; !ok {
				st.Rest = append(st.Rest, appcore.RestEntry{I: e.ThreadID, T: e.T})
			}
		}
		// 失敗したアカウントで何も取得できていないなら、先頭からの再試行を印す（尽きたアカウントと区別する）。
		st.Start = w.err != nil && st.PT == "" && len(st.Rest) == 0
		next[w.acc.ID] = st
	}
	page.NextCursor = appcore.EncodeSearchCursor(next)
	return page, nil
}

func entryLess(a, b searchEntry) bool {
	if a.T != b.T {
		return a.T > b.T
	}
	if a.AccountID != b.AccountID {
		return a.AccountID < b.AccountID
	}
	return a.ThreadID < b.ThreadID
}

func (s *InboxService) failure(ctx context.Context, accountID string, err error) dto.AccountFailure {
	return dto.AccountFailure{AccountID: accountID, Error: apperr.ToMap(gmailErr(s.d, ctx, accountID, err))}
}

// loadAccount は残り（カーソル由来）を起点に、limit 件に満ちるまで Gmail の次ページを取得して w.queue を作る。
// 失敗したときも w.queue と w.pt は「次の呼び出しがそこから再開できる状態」（入ってきた状態＋失敗前に取り込めたページ）に保つ。
func (s *InboxService) loadAccount(ctx context.Context, w *accountSearch, query string, limit int, st appcore.SearchState) {
	defer logging.RecoverTo(logger(s.d), "inbox search account", &w.err) // panic もこのアカウントの取得失敗にする（別 goroutine で動く）
	for _, r := range st.Rest {
		w.queue = append(w.queue, searchEntry{AccountID: w.acc.ID, ThreadID: r.I, T: r.T})
	}
	w.pt = st.PT
	labels, err := s.d.Store.ListLabels(ctx, w.acc.ID)
	if err != nil {
		w.err = err
		return
	}
	for _, l := range labels {
		if l.Type == "user" {
			w.labels = append(w.labels, l)
		}
	}
	// 初回（カーソルなし）は pt が空でも先頭ページを取る。続きの呼び出しで pt が空なら末尾。
	first := st.PT == "" && len(st.Rest) == 0
	for round := 0; (first || w.pt != "") && len(w.queue) < limit && round < maxFetchRounds; round++ {
		first = false
		var tl gmail.ThreadList
		if tl, err = s.d.Gmail.ListThreads(ctx, w.acc.ID, gmail.ListQuery{Q: query, PageToken: w.pt, MaxResults: int64(limit)}); err != nil {
			w.err = err
			return
		}
		entries := make([]*searchEntry, len(tl.Threads))
		errs := make([]error, len(tl.Threads))
		appcore.ParallelDo(logger(s.d), len(tl.Threads), appcore.SearchParallelism, func(i int) {
			defer logging.RecoverTo(logger(s.d), "inbox search thread", &errs[i]) // panic も取得失敗にする
			var meta gmail.ThreadMeta
			if meta, errs[i] = s.d.Gmail.GetThreadMetadata(ctx, w.acc.ID, tl.Threads[i].ID); errs[i] == nil {
				if sum, ok := appcore.SummaryFromMeta(w.acc.ID, meta, w.labels); ok {
					entries[i] = &searchEntry{AccountID: w.acc.ID, ThreadID: meta.ID, T: sum.LastMessageAt, Sum: &sum}
				}
			}
		})
		for _, e := range errs {
			if e != nil && !errors.Is(e, gmail.ErrNotFound) {
				w.err = e // このページは取り込まず、w.pt もこのページの手前のまま（次回このページから取り直す）。
				return
			}
		}
		w.pt = tl.NextPageToken
		for _, e := range entries {
			if e != nil {
				w.queue = append(w.queue, *e)
			}
		}
	}
	sort.SliceStable(w.queue, func(i, j int) bool { return entryLess(w.queue[i], w.queue[j]) })
}

// hydratePicked は配信するエントリのうちサマリ未取得のもの（カーソル由来）を取得する。
// 消えたスレッド（404）は除き、それ以外の失敗は Failures に載せる（そのエントリは配信せず残りに戻る）。
func (s *InboxService) hydratePicked(ctx context.Context, work []*accountSearch, picked []searchEntry, page *dto.ThreadPage) {
	byID := map[string]*accountSearch{}
	for _, w := range work {
		byID[w.acc.ID] = w
	}
	var mu sync.Mutex
	failedAccounts := map[string]bool{}
	appcore.ParallelDo(logger(s.d), len(picked), appcore.SearchParallelism, func(i int) {
		e := &picked[i]
		if e.Sum != nil {
			return
		}
		meta, err := s.d.Gmail.GetThreadMetadata(ctx, e.AccountID, e.ThreadID)
		mu.Lock()
		defer mu.Unlock()
		switch {
		case err == nil && summaryOK(e, meta, byID[e.AccountID].labels):
		case err == nil || errors.Is(err, gmail.ErrNotFound):
			dropEntry(byID[e.AccountID], e.ThreadID)
		case !failedAccounts[e.AccountID]:
			failedAccounts[e.AccountID] = true
			byID[e.AccountID].gone = apperr.IsPermanent(err)
			page.Failures = append(page.Failures, s.failure(ctx, e.AccountID, err))
		}
	})
}

// dropEntry は消えたスレッドを作業キューから除く。
func dropEntry(w *accountSearch, threadID string) {
	w.queue = slices.DeleteFunc(w.queue, func(e searchEntry) bool { return e.ThreadID == threadID })
}

// summaryOK は meta から e.Sum を作る。下書きしか無いスレッドは false（呼び出し側が結果から除く）。
func summaryOK(e *searchEntry, meta gmail.ThreadMeta, labels []store.Label) bool {
	sum, ok := appcore.SummaryFromMeta(e.AccountID, meta, labels)
	if ok {
		e.Sum = &sum
	}
	return ok
}
