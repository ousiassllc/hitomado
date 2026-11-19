// Package searchcore は InboxService.Search の本体（アカウントごとの並列検索と、時刻順の統合・カーソル生成）。
package searchcore

import (
	"context"
	"errors"
	"slices"
	"sort"
	"sync"

	"github.com/ousiassllc/nefos-inbox/internal/app/appcore"
	"github.com/ousiassllc/nefos-inbox/internal/app/apperr"
	"github.com/ousiassllc/nefos-inbox/internal/app/deps"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
	"github.com/ousiassllc/nefos-inbox/internal/gmail"
	"github.com/ousiassllc/nefos-inbox/internal/logging"
	"github.com/ousiassllc/nefos-inbox/internal/store"
)

// searcher は 1 回の Search の実行に使う依存。
type searcher struct {
	d              *deps.Deps
	maxFetchRounds int
	meta           *MetaCache
}

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
	// partial は、ページの一部のスレッドだけ取得に失敗した（取れたスレッドは queue にある）ときの最初の失敗。
	// 取れなかったスレッドは failed に持ち、サマリ未取得のまま queue に積む（配信時に取り直し、配信できなければ続きのカーソルに残る）。
	partial error
	failed  []string
	gone    bool // 再試行しても直らない失敗（続きのカーソルに残さない）
}

// Params は Search の入力。検証（クエリ・limit・カーソルの形）と対象アカウントの解決は呼び出し側で済ませる。
type Params struct {
	Deps *deps.Deps
	// Accounts は検索の対象アカウント（一覧の順）。
	Accounts []store.Account
	// States はカーソルを復号した、アカウントごとの続きの状態。nil なら先頭ページ。
	States map[string]appcore.SearchState
	Query  string
	Limit  int
	// MaxFetchRounds は 1 アカウントが 1 回の呼び出しで Gmail から追加取得する最大ページ数（一覧が埋まらない場合の歯止め）。
	MaxFetchRounds int
	// Meta は threads.get の結果の再利用（nil なら使わない）。
	Meta *MetaCache
}

// Search は Gmail 検索クエリを各アカウントで並列実行し、最終メッセージ日時の降順で統合する（結果は保存しない）。
// カーソルはアカウントごとの pageToken と未配信の残りを埋め込んだ不透明文字列で、サーバー側に状態を持たない。
func Search(ctx context.Context, p Params) dto.ThreadPage {
	s := &searcher{d: p.Deps, maxFetchRounds: p.MaxFetchRounds, meta: p.Meta}
	query, limit, states := p.Query, p.Limit, p.States
	var work []*accountSearch
	for _, a := range p.Accounts {
		if states != nil {
			if _, ok := states[a.ID]; !ok {
				continue // 前のページまでに出し尽くしたアカウント
			}
		}
		work = append(work, &accountSearch{acc: a})
	}
	page := dto.ThreadPage{Items: []dto.ThreadSummary{}, Failures: []dto.AccountFailure{}}

	appcore.ParallelDo(s.d.Log(), len(work), len(work), func(i int) { s.loadAccount(ctx, work[i], query, limit, states[work[i].acc.ID]) })
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
	// 取り直したスレッドは仮の時刻で並べていたので、本当の時刻で並べ直す（同じページの中だけ。残りは仮の時刻のまま続きへ）。
	for i := range picked {
		if picked[i].Sum != nil {
			picked[i].T = picked[i].Sum.LastMessageAt
		}
	}
	sort.SliceStable(picked, func(i, j int) bool { return entryLess(picked[i], picked[j]) })
	delivered := map[string]struct{}{}
	for _, e := range picked {
		if e.Sum != nil {
			page.Items = append(page.Items, *e.Sum)
			delivered[e.AccountID+"/"+e.ThreadID] = struct{}{}
		}
	}
	for _, w := range work {
		// 一部のスレッドが取れなかったアカウント（取れたスレッドは配信する）。取り直しても配信できなかった分が残るときだけ失敗として知らせる。
		if w.partial != nil && w.err == nil && slices.ContainsFunc(w.failed, func(id string) bool { _, ok := delivered[w.acc.ID+"/"+id]; return !ok }) &&
			!slices.ContainsFunc(page.Failures, func(f dto.AccountFailure) bool { return f.AccountID == w.acc.ID }) {
			page.Failures = append(page.Failures, s.failure(ctx, w.acc.ID, w.partial))
		}
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
	return page
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

func (s *searcher) failure(ctx context.Context, accountID string, err error) dto.AccountFailure {
	return dto.AccountFailure{AccountID: accountID, Error: apperr.ToMap(s.d.GmailErr(ctx, accountID, err))}
}

// loadAccount は残り（カーソル由来）を起点に、limit 件に満ちるまで Gmail の次ページを取得して w.queue を作る。
// 失敗したときも w.queue と w.pt は「次の呼び出しがそこから再開できる状態」（入ってきた状態＋失敗前に取り込めたページ）に保つ。
func (s *searcher) loadAccount(ctx context.Context, w *accountSearch, query string, limit int, st appcore.SearchState) {
	defer logging.RecoverTo(s.d.Log(), "inbox search account", &w.err) // panic もこのアカウントの取得失敗にする（別 goroutine で動く）
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
	for round := 0; (first || w.pt != "") && len(w.queue) < limit && round < s.maxFetchRounds; round++ {
		first = false
		var tl gmail.ThreadList
		if tl, err = s.d.Gmail.ListThreads(ctx, w.acc.ID, gmail.ListQuery{Q: query, PageToken: w.pt, MaxResults: int64(limit)}); err != nil {
			w.err = err
			return
		}
		entries := make([]*searchEntry, len(tl.Threads))
		errs := make([]error, len(tl.Threads))
		appcore.ParallelDo(s.d.Log(), len(tl.Threads), appcore.SearchParallelism, func(i int) {
			defer logging.RecoverTo(s.d.Log(), "inbox search thread", &errs[i]) // panic も取得失敗にする
			var meta gmail.ThreadMeta
			if meta, errs[i] = s.threadMeta(ctx, w.acc.ID, tl.Threads[i]); errs[i] == nil {
				if sum, ok := appcore.SummaryFromMeta(w.acc.ID, meta, w.labels); ok {
					entries[i] = &searchEntry{AccountID: w.acc.ID, ThreadID: meta.ID, T: sum.LastMessageAt, Sum: &sum}
				}
			}
		})
		ok, firstErr := 0, error(nil)
		for _, e := range errs {
			switch {
			case e == nil || errors.Is(e, gmail.ErrNotFound):
				ok++
			case firstErr == nil || (apperr.IsPermanent(e) && !apperr.IsPermanent(firstErr)):
				firstErr = e // 再接続などが要る失敗を優先する
			}
		}
		if firstErr != nil && (ok == 0 || apperr.IsPermanent(firstErr)) {
			// 1 件も取れない、または再接続などが要る失敗なら、このページは取り込まず、w.pt もこのページの手前のまま（次回このページから取り直す）。
			w.err = firstErr
			return
		}
		if firstErr != nil && w.partial == nil {
			w.partial = firstErr // 一部だけ失敗（利用上限など）。取れたスレッドは使う。
		}
		w.pt = tl.NextPageToken
		for _, e := range entries {
			if e != nil {
				w.queue = append(w.queue, *e)
			}
		}
		// 取れなかったスレッドは、同じページで取れたスレッドのいちばん古い時刻で仮に並べ（配信時に取り直し、本当の時刻は取り直し後に決まる）、
		// 続きのカーソルの残りに載せる。ページのトークンは進むので、載せないと黙って欠ける。
		fallback, found := int64(0), false
		for _, e := range entries {
			if e != nil && (!found || e.T < fallback) {
				fallback, found = e.T, true
			}
		}
		for _, q := range w.queue {
			if !found || q.T < fallback {
				fallback, found = q.T, true
			}
		}
		for i, e := range errs {
			if e != nil && !errors.Is(e, gmail.ErrNotFound) {
				w.failed = append(w.failed, tl.Threads[i].ID)
				w.queue = append(w.queue, searchEntry{AccountID: w.acc.ID, ThreadID: tl.Threads[i].ID, T: fallback})
			}
		}
	}
	sort.SliceStable(w.queue, func(i, j int) bool { return entryLess(w.queue[i], w.queue[j]) })
}

// threadMeta は stub のスレッドの metadata を返す。threads.list の historyId と一致する取得済みの結果があれば Gmail を呼ばない。
func (s *searcher) threadMeta(ctx context.Context, accountID string, stub gmail.ThreadStub) (gmail.ThreadMeta, error) {
	if meta, ok := s.meta.get(accountID, stub.ID, stub.HistoryID); ok {
		return meta, nil
	}
	meta, err := s.d.Gmail.GetThreadMetadata(ctx, accountID, stub.ID)
	if err == nil {
		s.meta.put(accountID, meta)
	}
	return meta, err
}

// hydratePicked は配信するエントリのうちサマリ未取得のもの（カーソル由来）を取得する。
// 消えたスレッド（404）は除き、それ以外の失敗は Failures に載せる（そのエントリは配信せず残りに戻る）。
func (s *searcher) hydratePicked(ctx context.Context, work []*accountSearch, picked []searchEntry, page *dto.ThreadPage) {
	byID := map[string]*accountSearch{}
	for _, w := range work {
		byID[w.acc.ID] = w
	}
	var mu sync.Mutex
	failedAccounts := map[string]bool{}
	appcore.ParallelDo(s.d.Log(), len(picked), appcore.SearchParallelism, func(i int) {
		e := &picked[i]
		if e.Sum != nil {
			return
		}
		meta, err := s.threadMetadata(ctx, e.AccountID, e.ThreadID)
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

// threadMetadata は GetThreadMetadata を呼び、panic もそのスレッドの取得失敗（error）にする（別 goroutine で動く）。
// 捕捉しないと ParallelDo の安全網が握りつぶし、そのエントリは結果にも Failures にも載らず黙って残りへ戻る。
func (s *searcher) threadMetadata(ctx context.Context, accountID, threadID string) (meta gmail.ThreadMeta, err error) {
	defer logging.RecoverTo(s.d.Log(), "inbox search hydrate", &err)
	return s.d.Gmail.GetThreadMetadata(ctx, accountID, threadID)
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
