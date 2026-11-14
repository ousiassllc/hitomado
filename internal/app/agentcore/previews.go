package agentcore

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ousiassllc/hitomado/internal/domain/signature"
	"github.com/ousiassllc/hitomado/internal/domain/textconv"
	"github.com/ousiassllc/hitomado/internal/gmail"
	"github.com/ousiassllc/hitomado/internal/store"
)

// 下書きのプレビュー（本文の最初の数行。プレーンテキスト）は DB に保存せず、メモリだけで持つ（NF-S08）。
const (
	// previewCacheSize はメモリに持つプレビューの上限（項目数）。超えたら古いものから捨てる。
	previewCacheSize = 100
	// previewFetchConcurrency は ListItems が Gmail.GetDraft を同時に呼ぶ上限。
	previewFetchConcurrency = 4
	// previewFetchTimeout は GetDraft 1 回の上限、previewListTimeout は ListItems 1 回で下書きの取得に使う合計の上限。
	// 超えた分のプレビューは空で返す（一覧は失敗させない）。
	previewFetchTimeout = 3 * time.Second
	previewListTimeout  = 5 * time.Second
	// previewRetryAfter は取得に失敗した項目を取り直すまでの間隔。
	previewRetryAfter = 5 * time.Minute
)

// DraftReader は Gmail の下書きの読み取り口（gmail.Client の GetDraft だけ。送信・作成・更新・削除は持たない）。
type DraftReader interface {
	GetDraft(ctx context.Context, accountID, draftID string) (gmail.DraftFull, error)
}

// previewEntry はキャッシュの 1 件。retryAt が 0 でなければ「取得に失敗した」記録（負のキャッシュ。text は空）で、retryAt までは取り直さない。
type previewEntry struct {
	draftID, text string
	retryAt       time.Time
}

// previewCache は項目 ID をキーにした、下書きのプレビューのメモリキャッシュ（上限つき。古いものから捨てる）。
// エントリは作ったときの DraftID を持ち、項目の DraftID が変わった（下書きを作り直した）ら使わない。
type previewCache struct {
	mu      sync.Mutex
	entries map[string]previewEntry
	order   []string // 挿入順（先頭が最古）
}

func newPreviewCache() *previewCache { return &previewCache{entries: map[string]previewEntry{}} }

// get はキャッシュにあるプレビューを返す。取得に失敗した記録が有効な間は ("", true)（取り直さない）。
func (c *previewCache) get(itemID, draftID string, now time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[itemID]
	if !ok || e.draftID != draftID || (!e.retryAt.IsZero() && !now.Before(e.retryAt)) {
		return "", false
	}
	return e.text, true
}

func (c *previewCache) put(itemID, draftID, text string) {
	c.set(itemID, previewEntry{draftID: draftID, text: text})
}

// putFailed は取得に失敗したことを retryAt まで覚える（一覧のたびに Gmail を叩かない）。
func (c *previewCache) putFailed(itemID, draftID string, retryAt time.Time) {
	c.set(itemID, previewEntry{draftID: draftID, retryAt: retryAt})
}

func (c *previewCache) set(itemID string, e previewEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[itemID]; !ok {
		c.order = append(c.order, itemID)
		if len(c.order) > previewCacheSize {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.entries[itemID] = e
}

// hasDraftPreview は項目がプレビューを出す対象か（ready／later で下書きがあるもの。パネルは done を描かない）。
func hasDraftPreview(it store.AgentItem) bool {
	return it.DraftID != "" && (it.Status == store.AgentStatusReady || it.Status == store.AgentStatusLater)
}

type fetchResult int

const (
	fetchOK     fetchResult = iota
	fetchGone               // 下書きが Gmail に無い（削除された・スレッド画面から送信された）
	fetchFailed             // それ以外の失敗（通信・時間切れなど）
)

// previewsFor は items のうちプレビューを出す対象の項目 ID → プレビューと、下書きが Gmail に無かった項目を返す。
// エージェントがオフのときは Gmail に問い合わせず、プレビューは空。メモリに無いものは Gmail.GetDraft で作る
// （並行度は previewFetchConcurrency。合計は previewListTimeout まで）。取得に失敗・超過したものは空で返し（失敗は previewRetryAfter の間、
// 取り直さない）、エラーにはしない。
func (s *Service) previewsFor(ctx context.Context, items []store.AgentItem) (map[string]string, []store.AgentItem) {
	out := map[string]string{}
	if s.d.Gmail == nil || !s.enabled(ctx) {
		return out, nil
	}
	now := s.d.Clock.Now()
	var missing []store.AgentItem
	for _, it := range items {
		if !hasDraftPreview(it) {
			continue
		}
		if text, ok := s.previews.get(it.ID, it.DraftID, now); ok {
			out[it.ID] = text
		} else {
			missing = append(missing, it)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}

	ctx, cancel := context.WithTimeout(ctx, previewListTimeout)
	defer cancel()
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		gone []store.AgentItem
		sem  = make(chan struct{}, previewFetchConcurrency)
	)
	for _, it := range missing {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break // 合計の締め切りを過ぎた分は、プレビュー空のまま
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			text, res := s.fetchPreview(ctx, it)
			switch {
			case res == fetchOK:
				s.previews.put(it.ID, it.DraftID, text)
				mu.Lock()
				out[it.ID] = text
				mu.Unlock()
			case res == fetchGone:
				mu.Lock()
				gone = append(gone, it)
				mu.Unlock()
			case ctx.Err() == nil: // 合計の締め切り・呼び出し元の中止による失敗は覚えない
				s.previews.putFailed(it.ID, it.DraftID, now.Add(previewRetryAfter))
			}
		}()
	}
	wg.Wait()
	return out, gone
}

// fetchPreview は Gmail の下書きから冒頭のプレビューを作る。失敗はアカウント ID とエラーの種類だけをログに残す。
func (s *Service) fetchPreview(ctx context.Context, it store.AgentItem) (string, fetchResult) {
	ctx, cancel := context.WithTimeout(ctx, previewFetchTimeout)
	defer cancel()
	d, err := s.d.Gmail.GetDraft(ctx, it.AccountID, it.DraftID)
	switch {
	case errors.Is(err, gmail.ErrNotFound):
		s.log.Info("agent: draft gone", "account", it.AccountID)
		return "", fetchGone
	case err != nil:
		s.log.Warn("agent: draft preview failed", "account", it.AccountID, "kind", draftErrKind(err))
		return "", fetchFailed
	}
	return preview(draftLeadText(d)), fetchOK
}

// draftErrKind は GetDraft の失敗を、ログに出せる固定の種類にする（エラーの内容は含めない。apperr.FromInfra はログを書くので使わない）。
func draftErrKind(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, gmail.ErrRateLimited):
		return "rate_limited"
	}
	return errKind(err)
}

// finishGone は下書きが Gmail から無くなった（削除された・スレッド画面から送信された）項目を、確認済み（done）にして保存する。
// 同じ項目を利用者が操作中のときは何もしない（次の一覧でもう一度確かめる）。保存できたら真。
func (s *Service) finishGone(ctx context.Context, it store.AgentItem) bool {
	if !s.begin(it.ID) {
		return false
	}
	defer s.end(it.ID)
	return s.markDone(ctx, it)
}

// markDone は it（取得時の状態）の下書きがまだ同じ ready／later なら、done にして保存し agent:updated を発行する。
// 項目の操作の排他（begin）は呼び出し側が持つ。保存できたら真。
func (s *Service) markDone(ctx context.Context, it store.AgentItem) bool {
	cur, err := s.d.Store.GetAgentItem(ctx, it.ID)
	if err != nil || cur.DraftID != it.DraftID || !hasDraftPreview(cur) {
		return false
	}
	cur.Status = store.AgentStatusDone
	if _, err := s.d.Store.UpdateAgentItem(ctx, cur); err != nil {
		s.log.Warn("agent: finish gone draft failed", "account", it.AccountID, "kind", errKind(err))
		return false
	}
	s.emitUpdatedNow(ctx)
	return true
}

// draftLeadText は下書きの本文のうち、署名・引用より前（AI が書いた部分）をプレーンテキストにする。
func draftLeadText(d gmail.DraftFull) string {
	html := d.Message.BodyHTML
	if html == "" {
		return d.Message.BodyText
	}
	for _, marker := range []string{signature.Marker, signature.QuoteMarker} {
		if at := strings.Index(html, marker); at >= 0 {
			html = html[:max(strings.LastIndex(html[:at], "<"), 0)] // マーカーを持つ開始タグの手前まで
		}
	}
	return textconv.HTMLToText(html)
}
